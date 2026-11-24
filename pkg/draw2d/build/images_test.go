//go:build !js

package build_test

import (
	"bytes"
	"encoding/json"
	"go/parser"
	"go/token"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/karty-game/ashura/pkg/assetbuild"
	spritebuild "github.com/karty-game/ashura/pkg/draw2d/build"
)

func TestSpriteImagesAtlasPreservesAlphaAndReusesDeterministicBuild(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	pixels := map[string]color.NRGBA{
		"Red": {R: 201, G: 129, B: 17, A: 64}, "Blue": {G: 240, B: 130, A: 128},
	}
	writePNG(t, filepath.Join(root, "assets/red.png"), pixels["Red"])
	writePNG(t, filepath.Join(root, "assets/blue.png"), pixels["Blue"])

	config := assetbuild.Config{Output: "generated/assets",
		Pipelines: []assetbuild.Pipeline{{Name: "fixture",
			Input: []string{"assets/**"},
			Import: []assetbuild.Plugin{
				assetbuild.Images(),
				assetbuild.Atlas(assetbuild.AtlasOptions{Name: "sprites", Include: []string{"**"}, Clamp: true, MaxSize: 16, Padding: 2}),
				spritebuild.Images(),
			},
		}},
	}
	options := assetbuild.Options{Project: root, Identity: "draw2d-image-pipeline-fixture"}

	report, err := assetbuild.Build(t.Context(), config, options)
	if err != nil || report.Executed != 4 {
		t.Fatal("cold build", report, err)
	}

	output := filepath.Join(root, "generated/assets")
	code := readFile(t, filepath.Join(output, "assets.go"))

	if _, err := parser.ParseFile(token.NewFileSet(), "assets.go", code, parser.AllErrors); err != nil {
		t.Fatal(err)
	}

	if !bytes.Contains(code, []byte("type Catalog struct")) || !bytes.Contains(code, []byte("func Load() (Catalog, error)")) ||
		bytes.Count(code, []byte("draw2d.NewImage(")) != 1 || bytes.Count(code, []byte(".Region(")) != 2 {
		t.Fatal("catalog did not share the page between typed regions", string(code))
	}

	metadata := readFile(t, filepath.Join(output, "atlas.json"))

	var regions []assetbuild.AtlasRegion
	if err := json.Unmarshal(metadata, &regions); err != nil {
		t.Fatal(err)
	}

	if len(regions) != 2 {
		t.Fatal("source region metadata is incomplete", regions)
	}

	page := readFile(t, filepath.Join(output, "pages", regions[0].Page+".rgba"))
	for _, region := range regions {
		first := (region.Y*region.PageWidth + region.X) * 4
		pixel := pixels[region.Name]

		if region.Width != 1 || region.Height != 1 || !bytes.Equal(page[first:first+4], []byte{pixel.R, pixel.G, pixel.B, pixel.A}) {
			t.Fatal("source RGB/alpha changed while packing", region, page[first:first+4], pixel)
		}
		// Extruded padding preserves the same straight-alpha source pixels.
		if !bytes.Equal(page[first-4:first], page[first:first+4]) {
			t.Fatal("atlas gutter changed source alpha", region)
		}
	}

	report, err = assetbuild.Build(t.Context(), config, options)
	if err != nil || report.Executed != 0 || report.Reused != 4 || report.Written != 0 {
		t.Fatal("warm build reprocessed or rewrote unchanged assets", report, err)
	}

	options.NoCache = true
	if _, err := assetbuild.Build(t.Context(), config, options); err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(code, readFile(t, filepath.Join(output, "assets.go"))) ||
		!bytes.Equal(metadata, readFile(t, filepath.Join(output, "atlas.json"))) ||
		!bytes.Equal(page, readFile(t, filepath.Join(output, "pages", regions[0].Page+".rgba"))) {
		t.Fatal("uncached rebuild changed prepared bytes or metadata")
	}

	options.NoCache = false

	writePNG(t, filepath.Join(root, "assets/red.png"), color.NRGBA{R: 30, G: 90, B: 150, A: 192})

	report, err = assetbuild.Build(t.Context(), config, options)
	if err != nil || report.Executed != 3 || report.Reused != 1 {
		t.Fatal("editing one PNG did not update its atlas and catalog", report, err)
	}

	if bytes.Equal(page, readFile(t, filepath.Join(output, "pages", regions[0].Page+".rgba"))) ||
		!bytes.Equal(code, readFile(t, filepath.Join(output, "assets.go"))) ||
		!bytes.Equal(metadata, readFile(t, filepath.Join(output, "atlas.json"))) {
		t.Fatal("pixel-only edit changed region layout or failed to update prepared page")
	}
}

func TestSpriteImagesRejectsEncodedTexturesAndMipChains(t *testing.T) {
	t.Parallel()

	for _, stage := range []string{"ktx2", "mipmaps"} {
		t.Run(stage, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			writePNGSize(t, filepath.Join(root, "assets/icon.png"), color.NRGBA{R: 80, A: 255}, 2)

			processor := assetbuild.KTX2()
			if stage == "mipmaps" {
				processor = assetbuild.Mipmaps()
			}

			config := assetbuild.Config{Output: "generated/assets",
				Pipelines: []assetbuild.Pipeline{{Name: "fixture",
					Input:  []string{"assets/**"},
					Import: []assetbuild.Plugin{assetbuild.Images(), processor, spritebuild.Images()},
				}},
			}
			if _, err := assetbuild.Build(
				t.Context(),
				config,
				assetbuild.Options{Project: root, Identity: "draw2d-invalid-stage"},
			); err == nil {
				t.Fatal("unsupported image processing stage was silently accepted")
			}

			if _, err := os.Stat(filepath.Join(root, "generated/assets/assets.go")); !os.IsNotExist(err) {
				t.Fatal("failed recipe published a catalog", err)
			}
		})
	}
}

func writePNG(t *testing.T, name string, pixel color.NRGBA) {
	t.Helper()
	writePNGSize(t, name, pixel, 1)
}

func writePNGSize(t *testing.T, name string, pixel color.NRGBA, size int) {
	t.Helper()

	source := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := range size {
		for x := range size {
			source.SetNRGBA(x, y, pixel)
		}
	}

	var data bytes.Buffer
	if err := png.Encode(&data, source); err != nil {
		t.Fatal(err)
	}

	if err := os.MkdirAll(filepath.Dir(name), 0o750); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(name, data.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, name string) []byte {
	t.Helper()

	//nolint:gosec // Paths are constructed entirely within the test-owned temporary fixture.
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}

	return data
}

func TestSpriteImagesStandaloneAndFailedSourcePreservePublishedOutput(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	input := filepath.Join(root, "assets/icon.png")
	writePNGSize(t, input, color.NRGBA{R: 240, G: 80, B: 20, A: 128}, 32)

	config := assetbuild.Config{Output: "generated/assets",
		Pipelines: []assetbuild.Pipeline{{Name: "fixture",
			Input:  []string{"assets/**"},
			Import: []assetbuild.Plugin{assetbuild.Images(), spritebuild.Images()},
		}},
	}

	options := assetbuild.Options{Project: root, Identity: "sprite-standalone@1"}
	if _, err := assetbuild.Build(t.Context(), config, options); err != nil {
		t.Fatal(err)
	}

	output := filepath.Join(root, "generated/assets")
	code := readFile(t, filepath.Join(output, "assets.go"))

	page := readFile(t, filepath.Join(output, "pages/Icon.rgba"))
	if len(page) != 32*32*4 || !bytes.Equal(page[:4], []byte{240, 80, 20, 128}) ||
		!bytes.Contains(code, []byte("draw2d.NewImage(32, 32, imagePage0)")) || bytes.Contains(code, []byte(".Region(")) {
		t.Fatal("standalone preparation changed size, straight alpha, or region ownership", string(code))
	}

	if _, err := os.Stat(filepath.Join(output, "atlas.json")); !os.IsNotExist(err) {
		t.Fatal("standalone pipeline published atlas metadata", err)
	}

	if err := os.WriteFile(input, []byte("broken PNG"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := assetbuild.Build(t.Context(), config, options); err == nil {
		t.Fatal("corrupt source image was accepted")
	}

	if !bytes.Equal(code, readFile(t, filepath.Join(output, "assets.go"))) ||
		!bytes.Equal(page, readFile(t, filepath.Join(output, "pages/Icon.rgba"))) {
		t.Fatal("failed source admission changed previously published assets")
	}
}
