//go:build !js

package assetbuild_test

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/karty-game/ashura/internal/ktx2data"
	"github.com/karty-game/ashura/pkg/assetbuild"
	uibuild "github.com/karty-game/ashura/pkg/ui/build"
)

func TestPipelineImageExportersKeepIndependentRulesAndCaches(t *testing.T) {
	t.Parallel()
	project := t.TempDir()
	pipelineImageSource(t, project)

	config := assetbuild.Config{Output: "generated", Pipelines: []assetbuild.Pipeline{
		{
			Name: "ui", Directory: "ui", Input: []string{"assets/**"},
			Import:  []assetbuild.Plugin{assetbuild.Images()},
			Process: []assetbuild.Plugin{assetbuild.Resize(assetbuild.ResizeOptions{Width: 2, Height: 1})},
			Output:  []assetbuild.Plugin{uibuild.Images()},
		},
		{
			Name: "textures", Directory: "textures", Input: []string{"assets/**"},
			Import: []assetbuild.Plugin{assetbuild.Images()},
			Process: []assetbuild.Plugin{
				assetbuild.Resize(assetbuild.ResizeOptions{Width: 4, Height: 2}),
				assetbuild.Mipmaps(),
			},
			Output: []assetbuild.Plugin{assetbuild.KTX2(), assetbuild.GoCatalog()},
		},
	}}

	options := assetbuild.Options{Project: project, Identity: "independent-image-pipelines@1", Jobs: 2}
	if _, err := assetbuild.Build(t.Context(), config, options); err != nil {
		t.Fatal(err)
	}

	pipelineUIPage(t, project, 2)
	texturePath := filepath.Join(project, "generated/textures/shared-albedo.ktx2")
	encoded := pipelineImageRead(t, texturePath)

	data, err := ktx2data.Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}

	if data.Width != 4 || data.Height != 2 || len(data.Levels) != 3 {
		t.Fatalf("texture pipeline lost its independent resize/mips: %+v", data)
	}

	for index, level := range data.Levels {
		if !bytes.Equal(level, bytes.Repeat([]byte{0, 255, 0, 255}, len(level)/4)) {
			t.Fatalf("texture mip %d changed the shared source's opaque green pixels", index)
		}
	}

	catalog := pipelineImageRead(t, filepath.Join(project, "generated/textures/assets.go"))
	if !bytes.Contains(catalog, []byte("func SharedAlbedo()")) || bytes.Contains(catalog, []byte("func Load()")) {
		t.Fatalf("texture catalog was replaced by the UI exporter: %s", catalog)
	}

	// Keep the recipe identity fixed: only the UI processing option changes.
	// Both branches import the same logical resource and publish assets.go.
	config.Pipelines[0].Process = []assetbuild.Plugin{assetbuild.Resize(assetbuild.ResizeOptions{Width: 1, Height: 1})}

	report, err := assetbuild.Build(t.Context(), config, options)
	if err != nil {
		t.Fatal(err)
	}

	pipelineImageCacheReuse(t, report)
	pipelineUIPage(t, project, 1)

	if !bytes.Equal(encoded, pipelineImageRead(t, texturePath)) ||
		!bytes.Equal(catalog, pipelineImageRead(t, filepath.Join(project, "generated/textures/assets.go"))) {
		t.Fatal("UI-only processing changed the independent texture payload or catalog")
	}
}

func pipelineImageCacheReuse(t *testing.T, report assetbuild.Report) {
	t.Helper()

	var textureHits, uiMisses int

	for _, action := range report.Actions {
		if strings.HasPrefix(action.ID, "pipeline/textures/") {
			if action.Status != "HIT" {
				t.Fatalf("UI-only resize invalidated the independent texture branch: %+v", action)
			}

			textureHits++
		}

		if strings.HasPrefix(action.ID, "pipeline/ui/") && action.Status == "MISS" {
			uiMisses++
		}
	}

	if textureHits != 5 || uiMisses != 2 {
		t.Fatalf("expected all five texture actions reused and UI resize/export rebuilt: %+v", report)
	}
}

func pipelineUIPage(t *testing.T, project string, width int) {
	t.Helper()

	pixels := pipelineImageRead(t, filepath.Join(project, "generated/ui/pages/SharedAlbedo.rgba"))
	if !bytes.Equal(pixels, bytes.Repeat([]byte{0, 255, 0, 255}, width)) {
		t.Fatalf("UI page does not contain exactly %d opaque green pixels: %v", width, pixels)
	}

	catalog := pipelineImageRead(t, filepath.Join(project, "generated/ui/assets.go"))

	expected := "ui.NewImageAsset(2, 1,"
	if width == 1 {
		expected = "ui.NewImageAsset(1, 1,"
	}

	if !bytes.Contains(catalog, []byte(expected)) || !bytes.Contains(catalog, []byte("SharedAlbedo *ui.ImageAsset")) {
		t.Fatalf("UI catalog lost its independent dimensions or declaration: %s", catalog)
	}
}

func pipelineImageSource(t *testing.T, project string) {
	t.Helper()

	pixels := image.NewNRGBA(image.Rect(0, 0, 8, 4))

	for y := range 4 {
		for x := range 8 {
			pixels.SetNRGBA(x, y, color.NRGBA{G: 255, A: 255})
		}
	}

	var encoded bytes.Buffer
	if err := png.Encode(&encoded, pixels); err != nil {
		t.Fatal(err)
	}

	if err := os.Mkdir(filepath.Join(project, "assets"), 0o700); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(project, "assets/shared-albedo.png"), encoded.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}

func pipelineImageRead(t *testing.T, path string) []byte {
	t.Helper()
	//nolint:gosec // G304: callers supply fixed filenames under this test's private directory.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	return data
}
