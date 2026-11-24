//go:build !js

package build_test

import (
	"bytes"
	"image/color"
	"path/filepath"
	"strings"
	"testing"

	"github.com/karty-game/ashura/pkg/assetbuild"
	spritebuild "github.com/karty-game/ashura/pkg/draw2d/build"
	"github.com/karty-game/ashura/pkg/texture"
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

	pageOptions.OpaqueFormat = texture.ETC2RGB

	pageOptions.AlphaFormat = texture.ETC2RGBA
	if _, err := assetbuild.Build(t.Context(), compressedSpriteConfig(pageOptions), options); err != nil {
		t.Fatal("prepare mobile sprite pages", err)
	}

	if len(readFile(t, filepath.Join(output, "pages/Background.etc2rgb"))) != 32 ||
		len(readFile(t, filepath.Join(output, "pages/Glow.etc2rgba"))) != 64 {
		t.Fatal("mobile page storage changed dimensions or alpha format")
	}

	runGeneratedSpriteCatalog(t, root, strings.Replace(compressedRuntimeTest,
		"[]texture.Format{texture.BC7, texture.BC7}", "[]texture.Format{texture.ETC2RGB, texture.ETC2RGBA}", 1))

	report, err = assetbuild.Build(t.Context(), compressedSpriteConfig(pageOptions), options)
	if err != nil || report.Executed != 0 || report.Written != 0 {
		t.Fatal("warm mobile build repeated preparation", report, err)
	}
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
		{"alpha-etc2rgb", 4, spritebuild.ImagesOptions{AlphaFormat: texture.ETC2RGB}},
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

    "github.com/karty-game/ashura/pkg/draw2d"
    "github.com/karty-game/ashura/pkg/texture"
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
    formats := []texture.Format{texture.BC7, texture.BC7}
    for index, batch := range frame.Batches {
        page := batch.Texture
        expectedBytes := 64
        if formats[index] == texture.ETC2RGB { expectedBytes = 32 }
        if page.Width != 8 || page.Height != 8 || page.Format != formats[index] || len(page.Pixels) != expectedBytes {
            t.Fatal("compressed page metadata or payload changed")
        }
    }
    original := imagePage0
    defer func() { imagePage0 = original }()
    imagePage0 = original[:len(original)-1]
    if _, err := Load(); err == nil { t.Fatal("truncated compressed page admitted") }
}
`
