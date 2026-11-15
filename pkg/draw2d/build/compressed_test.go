//go:build !js

package build_test

import (
	"bytes"
	"image/color"
	"path/filepath"
	"testing"

	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
	spritebuild "github.com/karty-game/karty-webgpu/pkg/draw2d/build"
	"github.com/karty-game/karty-webgpu/pkg/texture"
)

func TestCompressedSpritePagesPreserveDimensionsAndReuseBuild(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writePNGSize(t, filepath.Join(root, "assets/background.png"), color.NRGBA{R: 80, G: 140, B: 210, A: 255}, 8)
	writePNGSize(t, filepath.Join(root, "assets/glow.png"), color.NRGBA{R: 230, G: 160, B: 90, A: 96}, 8)

	pageOptions := spritebuild.ImagesOptions{OpaqueFormat: texture.BC1, AlphaFormat: texture.BC7}
	config := compressedSpriteConfig(pageOptions)

	options := assetbuild.Options{Project: root, Identity: "compressed-sprite-pages@1"}
	if _, err := assetbuild.Build(t.Context(), config, options); err != nil {
		t.Fatal(err)
	}

	output := filepath.Join(root, "generated/assets")
	background := readFile(t, filepath.Join(output, "pages/Background.bc1"))
	glow := readFile(t, filepath.Join(output, "pages/Glow.bc7"))

	code := readFile(t, filepath.Join(output, "assets.go"))
	if len(background) != 32 || len(glow) != 64 ||
		!bytes.Contains(code, []byte("draw2d.NewPreparedImage(8, 8, texture.BC1, imagePage0)")) ||
		!bytes.Contains(code, []byte("draw2d.NewPreparedImage(8, 8, texture.BC7, imagePage1)")) {
		t.Fatal("compressed payload, dimensions or alpha-based format selection changed", len(background), len(glow), string(code))
	}

	report, err := assetbuild.Build(t.Context(), config, options)
	if err != nil || report.Executed != 0 || report.Reused != 3 || report.Written != 0 {
		t.Fatal("warm build repeated encoding or publication", report, err)
	}

	pageOptions.OpaqueFormat = texture.BC7

	report, err = assetbuild.Build(t.Context(), compressedSpriteConfig(pageOptions), options)
	if err != nil || report.Executed != 1 || report.Reused != 2 {
		t.Fatal("format options did not invalidate only page preparation", report, err)
	}

	if len(readFile(t, filepath.Join(output, "pages/Background.bc7"))) != 64 {
		t.Fatal("changed opaque format did not update published payload")
	}

	runGeneratedSpriteCatalog(t, root, compressedRuntimeTest)
}

func TestCompressedSpritePagesRejectMisalignedSourceAndInvalidFormats(t *testing.T) {
	t.Parallel()

	for _, fixture := range []struct {
		name    string
		size    int
		options spritebuild.ImagesOptions
	}{
		{"misaligned", 3, spritebuild.ImagesOptions{OpaqueFormat: texture.BC1}},
		{"alpha-bc1", 4, spritebuild.ImagesOptions{AlphaFormat: texture.BC1}},
		{"unsupported", 4, spritebuild.ImagesOptions{OpaqueFormat: texture.BC6H}},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			writePNGSize(t, filepath.Join(root, "assets/icon.png"), color.NRGBA{R: 80, A: 255}, fixture.size)

			if _, err := assetbuild.Build(t.Context(), compressedSpriteConfig(fixture.options),
				assetbuild.Options{Project: root, Identity: "invalid-compressed-sprites@1"}); err == nil {
				t.Fatal("unsupported page configuration was accepted")
			}
		})
	}
}

func compressedSpriteConfig(options spritebuild.ImagesOptions) assetbuild.Config {
	return assetbuild.Config{Output: "generated/assets", Pipelines: []assetbuild.Pipeline{{
		Name: "fixture", Input: []string{"assets/**"}, Import: []assetbuild.Plugin{assetbuild.Images()},
		Output: []assetbuild.Plugin{spritebuild.Images(options)},
	}}}
}

const compressedRuntimeTest = `package assets

import (
    "testing"

    "github.com/karty-game/karty-webgpu/pkg/draw2d"
    "github.com/karty-game/karty-webgpu/pkg/texture"
)

func TestCompressedCatalog(t *testing.T) {
    images, err := Load()
    if err != nil { t.Fatal(err) }
    screen := draw2d.NewScreen()
    defer screen.Close()
    screen.NewSprite(images.Background)
    screen.NewSprite(images.Glow)
    frame := screen.Frame(32,32)
    if len(frame.Batches) != 2 { t.Fatal("distinct compressed pages lost", len(frame.Batches)) }
    for _, batch := range frame.Batches {
        page := batch.Texture
        if page.Width != 8 || page.Height != 8 || page.Format != texture.BC7 || len(page.Pixels) != 64 {
            t.Fatal("compressed page metadata or payload changed")
        }
    }
    original := imagePage0
    defer func() { imagePage0 = original }()
    imagePage0 = original[:len(original)-1]
    if _, err := Load(); err == nil { t.Fatal("truncated compressed page admitted") }
}
`
