//go:build !js

package assetbuild_test

import (
	"bytes"
	"go/parser"
	"go/token"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
	"github.com/karty-game/karty-webgpu/pkg/texture/ktx2"
)

func TestImagePluginsResizeCacheAndIdenticalDecode(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	input := filepath.Join(root, "assets", "brick-albedo.png")
	source := stripedImage(4, 2)
	writePluginPNG(t, input, source, png.DefaultCompression)

	config := assetbuild.Config{
		Input:  []string{"assets/**"},
		Output: "generated/assets",
		Plugins: []assetbuild.Plugin{
			assetbuild.Images(),
			assetbuild.Resize(assetbuild.ResizeOptions{Width: 2, Height: 1}),
			assetbuild.Mipmaps(),
			assetbuild.KTX2(),
			assetbuild.GoCatalog(),
		},
	}
	options := assetbuild.Options{Project: root, Identity: "image-plugins-fixture"}

	report, err := assetbuild.Build(t.Context(), config, options)
	if err != nil {
		t.Fatal(err)
	}

	if report.Executed != 5 {
		t.Fatalf("cold processing = %+v", report)
	}

	output := filepath.Join(root, "generated", "assets")
	prepared := pluginRead(t, filepath.Join(output, "brick-albedo.ktx2"))

	data, err := ktx2.Decode(prepared)
	if err != nil {
		t.Fatal(err)
	}

	if data.Width != 2 || data.Height != 1 || len(data.Levels) != 2 || data.Levels[0][0] != 188 {
		t.Fatalf("resize must average in linear light and author mips: %+v", data)
	}

	code := pluginRead(t, filepath.Join(output, "assets.go"))
	if !bytes.Contains(code, []byte("func BrickAlbedo()")) || bytes.Contains(code, []byte("func AssetsBrickAlbedo()")) {
		t.Fatalf("source-relative declaration missing: %s", code)
	}

	if _, err := parser.ParseFile(token.NewFileSet(), "assets.go", code, parser.AllErrors); err != nil {
		t.Fatal(err)
	}

	fixed := time.Unix(1_600_000_000, 0)
	for _, name := range []string{"brick-albedo.ktx2", "assets.go", ".engine-assets.json"} {
		if err := os.Chtimes(filepath.Join(output, name), fixed, fixed); err != nil {
			t.Fatal(err)
		}
	}

	report, err = assetbuild.Build(t.Context(), config, options)
	if err != nil || report.Executed != 0 || report.Reused != 5 {
		t.Fatalf("warm build: %+v, %v", report, err)
	}

	for _, name := range []string{"brick-albedo.ktx2", "assets.go", ".engine-assets.json"} {
		info, statErr := os.Stat(filepath.Join(output, name))
		if statErr != nil || !info.ModTime().Equal(fixed) {
			t.Fatalf("unchanged %s rewritten: %v %v", name, info, statErr)
		}
	}

	writePluginPNG(t, input, source, png.BestSpeed)

	report, err = assetbuild.Build(t.Context(), config, options)
	if err != nil || report.Executed != 1 || report.Reused != 4 {
		t.Fatalf("identical decoded pixels should stop propagation: %+v, %v", report, err)
	}

	if !bytes.Equal(prepared, pluginRead(t, filepath.Join(output, "brick-albedo.ktx2"))) {
		t.Fatal("identical decoded pixels changed prepared output")
	}

	source.SetNRGBA(0, 0, color.NRGBA{R: 80, G: 80, B: 80, A: 255})
	writePluginPNG(t, input, source, png.BestSpeed)

	report, err = assetbuild.Build(t.Context(), config, options)
	if err != nil || report.Executed != 4 || report.Reused != 1 {
		t.Fatalf("pixel change should preserve catalog metadata cache: %+v, %v", report, err)
	}

	if !bytes.Equal(code, pluginRead(t, filepath.Join(output, "assets.go"))) {
		t.Fatal("payload-only change modified catalog code")
	}
}

func TestResizePreservesNumericRoles(t *testing.T) {
	t.Parallel()

	for _, usage := range []string{"normal", "ao", "ssbump"} {
		t.Run(usage, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			source := image.NewNRGBA(image.Rect(0, 0, 2, 1))

			pixel := color.NRGBA{R: 20, G: 60, B: 100, A: 255}
			if usage == "normal" {
				pixel = color.NRGBA{R: 255, G: 128, B: 255, A: 255}
			}

			source.SetNRGBA(0, 0, pixel)
			source.SetNRGBA(1, 0, pixel)
			writePluginPNG(t, filepath.Join(root, "assets", usage+".png"), source, png.DefaultCompression)

			config := assetbuild.Config{
				Input:   []string{"assets/**"},
				Output:  "generated/assets",
				Plugins: []assetbuild.Plugin{assetbuild.Images(), assetbuild.Resize(assetbuild.ResizeOptions{Width: 1}), assetbuild.KTX2()},
			}
			if _, err := assetbuild.Build(t.Context(), config, assetbuild.Options{Project: root, Identity: "numeric-resize"}); err != nil {
				t.Fatal(err)
			}

			data, err := ktx2.Decode(pluginRead(t, filepath.Join(root, "generated", "assets", usage+".ktx2")))
			if err != nil {
				t.Fatal(err)
			}

			if usage == "normal" {
				var length float64

				for _, channel := range data.Levels[0][:3] {
					value := float64(channel)/255*2 - 1
					length += value * value
				}

				if math.Abs(length-1) > .02 {
					t.Fatalf("resized normal not normalized: %v", data.Levels[0])
				}
			} else if !bytes.Equal(data.Levels[0], []byte{20, 60, 100, 255}) {
				t.Fatalf("numeric channels modified: %v", data.Levels[0])
			}
		})
	}
}

func TestResizeAspectAndInvalidOptions(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writePluginPNG(t, filepath.Join(root, "assets", "brick.png"), stripedImage(8, 4), png.DefaultCompression)
	options := assetbuild.Options{Project: root, Identity: "resize-options"}

	config := assetbuild.Config{
		Input:   []string{"assets/**"},
		Output:  "generated/assets",
		Plugins: []assetbuild.Plugin{assetbuild.Images(), assetbuild.Resize(assetbuild.ResizeOptions{MaxDimension: 4}), assetbuild.KTX2()},
	}
	if _, err := assetbuild.Build(t.Context(), config, options); err != nil {
		t.Fatal(err)
	}

	output := filepath.Join(root, "generated", "assets", "brick.ktx2")
	original := pluginRead(t, output)

	data, err := ktx2.Decode(original)
	if err != nil || data.Width != 4 || data.Height != 2 {
		t.Fatalf("aspect ratio not preserved: %+v %v", data, err)
	}

	for _, resize := range []assetbuild.ResizeOptions{{}, {Width: -1}, {Width: 4097}, {Width: 1, MaxDimension: 1}, {Width: 1, Include: []string{"ui/["}}} {
		config.Plugins[1] = assetbuild.Resize(resize)
		if _, err := assetbuild.Build(t.Context(), config, options); err == nil {
			t.Fatalf("accepted invalid options: %+v", resize)
		}

		if !bytes.Equal(original, pluginRead(t, output)) {
			t.Fatal("failed build changed published output")
		}
	}
}

func TestImagePluginOrderValidation(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writePluginPNG(t, filepath.Join(root, "assets", "brick.png"), stripedImage(2, 2), png.DefaultCompression)

	for _, plugins := range [][]assetbuild.Plugin{
		{assetbuild.Images(), assetbuild.GoCatalog()},
		{assetbuild.Images(), assetbuild.KTX2(), assetbuild.Mipmaps()},
		{assetbuild.Images(), assetbuild.KTX2(), assetbuild.Resize(assetbuild.ResizeOptions{Width: 1})},
	} {
		config := assetbuild.Config{Input: []string{"assets/**"}, Output: "generated/assets", Plugins: plugins}

		report, err := assetbuild.Build(t.Context(), config, assetbuild.Options{Project: root, Identity: "invalid-order"})
		if err == nil || report.Executed != 0 {
			t.Fatalf("invalid order should fail planning: %+v %v", report, err)
		}
	}
}

func stripedImage(width, height int) *image.NRGBA {
	result := image.NewNRGBA(image.Rect(0, 0, width, height))
	for row := range height {
		for column := range width {
			value := byte(0)
			if column%2 == 1 {
				value = 255
			}

			result.SetNRGBA(column, row, color.NRGBA{R: value, G: value, B: value, A: 255})
		}
	}

	return result
}

func writePluginPNG(t *testing.T, path string, source image.Image, compression png.CompressionLevel) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}

	var data bytes.Buffer

	encoder := png.Encoder{CompressionLevel: compression}
	if err := encoder.Encode(&data, source); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, data.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}

func pluginRead(t *testing.T, path string) []byte {
	t.Helper()
	//nolint:gosec // Callers pass only generated fixture paths confined to t.TempDir.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	return data
}
