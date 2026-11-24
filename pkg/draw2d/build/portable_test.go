//go:build !js

package build_test

import (
	"bytes"
	"go/parser"
	"go/token"
	"image/color"
	"path/filepath"
	"testing"

	"github.com/karty-game/ashura/pkg/assetbuild"
	spritebuild "github.com/karty-game/ashura/pkg/draw2d/build"
	"github.com/karty-game/ashura/pkg/texture"
)

func TestPortableSpriteImagesSharePreparationAndPreserveOutputs(t *testing.T) {
	t.Parallel()

	const output = "generated/assets"

	legacyRoot, portableRoot := t.TempDir(), t.TempDir()
	for _, root := range []string{legacyRoot, portableRoot} {
		writePNGSize(t, filepath.Join(root, "assets/background.png"), color.NRGBA{R: 250, A: 255}, 4)
		writePNGSize(t, filepath.Join(root, "assets/sprites/coin.png"), color.NRGBA{G: 250, A: 125}, 4)
	}

	legacy := assetbuild.Config{Output: output, Pipelines: []assetbuild.Pipeline{
		spriteAtlasPipeline(
			"sprites",
			"",
			spritebuild.Images(spritebuild.ImagesOptions{OpaqueFormat: texture.BC1, AlphaFormat: texture.BC7}),
		),
		spriteAtlasPipeline("spritesMobile", "mobile", spritebuild.Images(spritebuild.ImagesOptions{
			OpaqueFormat: texture.ETC2RGB, AlphaFormat: texture.ETC2RGBA,
		})),
	}}
	options := assetbuild.Options{Project: legacyRoot, Identity: "sprite-portable-fixture"}

	report, err := assetbuild.Build(t.Context(), legacy, options)
	if err != nil || report.Executed != 8 {
		t.Fatal("independent desktop and mobile fixture", report, err)
	}

	portable := assetbuild.Config{Output: output, Pipelines: []assetbuild.Pipeline{
		spriteAtlasPipeline("sprites", "", spritebuild.PortableImages()),
	}}
	options.Project = portableRoot

	report, err = assetbuild.Build(t.Context(), portable, options)
	if err != nil || report.Executed != 5 {
		t.Fatal("portable recipe should share the two imports and atlas", report, err)
	}

	for _, name := range []string{
		"assets.go", "atlas.json", "pages/Background.bc1", "pages/SpritesAlbedoPage0.bc7",
		"mobile/assets.go", "mobile/atlas.json", "mobile/pages/Background.etc2rgb", "mobile/pages/SpritesAlbedoPage0.etc2rgba",
	} {
		if !bytes.Equal(readFile(t, filepath.Join(legacyRoot, output, name)), readFile(t, filepath.Join(portableRoot, output, name))) {
			t.Fatal("portable recipe changed legacy output", name)
		}
	}

	code := readFile(t, filepath.Join(portableRoot, output, "portable.go"))
	if _, err := parser.ParseFile(token.NewFileSet(), "portable.go", code, parser.AllErrors); err != nil {
		t.Fatal(err)
	}

	report, err = assetbuild.Build(t.Context(), portable, options)
	if err != nil || report.Executed != 0 || report.Reused != 5 || report.Written != 0 {
		t.Fatal("warm portable recipe should reuse every action", report, err)
	}
}

func spriteAtlasPipeline(name, directory string, publisher assetbuild.Plugin) assetbuild.Pipeline {
	return assetbuild.Pipeline{
		Name: name, Directory: directory, Input: []string{"assets/**"},
		Import: []assetbuild.Plugin{assetbuild.Images()},
		Process: []assetbuild.Plugin{assetbuild.Atlas(assetbuild.AtlasOptions{
			Name: "sprites", Include: []string{"sprites/**"}, Clamp: true, MaxSize: 16, Padding: 2,
		})},
		Output: []assetbuild.Plugin{publisher},
	}
}
