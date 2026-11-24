//go:build !js

package build_test

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/karty-game/ashura/pkg/assetbuild"
	spritebuild "github.com/karty-game/ashura/pkg/draw2d/build"
)

func TestSpriteImagesGeneratedCatalogRunsWithMultiplePagesAndStandaloneImage(t *testing.T) {
	t.Parallel()

	project := t.TempDir()
	for _, source := range []struct {
		path          string
		width, height int
		pixel         color.NRGBA
	}{
		{"ui/small-a.png", 2, 2, color.NRGBA{R: 240, G: 80, B: 20, A: 128}},
		{"ui/small-b.png", 2, 2, color.NRGBA{R: 20, G: 180, B: 240, A: 96}},
		{"ui/big.png", 6, 6, color.NRGBA{R: 120, G: 240, B: 40, A: 255}},
		{"standalone.png", 3, 2, color.NRGBA{R: 255, G: 210, B: 50, A: 255}},
	} {
		reviewPNG(t, filepath.Join(project, "assets", source.path), source.width, source.height, source.pixel)
	}

	config := assetbuild.Config{Output: "generated/assets",
		Pipelines: []assetbuild.Pipeline{{Name: "fixture",
			Input: []string{"assets/**"},
			Import: []assetbuild.Plugin{
				assetbuild.Images(),
				assetbuild.Atlas(assetbuild.AtlasOptions{Name: "ui", Include: []string{"ui/**"}, Clamp: true, MaxSize: 8}),
				spritebuild.Images(),
			},
		}},
	}
	if _, err := assetbuild.Build(
		t.Context(),
		config,
		assetbuild.Options{Project: project, Identity: "draw2d-images-execution@1"},
	); err != nil {
		t.Fatal(err)
	}

	runGeneratedSpriteCatalog(t, project, reviewRuntimeTest)
}

func runGeneratedSpriteCatalog(t *testing.T, project, source string) {
	t.Helper()

	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}

	module := fmt.Sprintf(
		"module github.com/karty-game/ashura/internal/draw2d-recipe-fixture\n\ngo 1.27.0\n\nrequire github.com/karty-game/ashura v0.0.0\n\nreplace github.com/karty-game/ashura => %q\n",
		root,
	)
	if err = os.WriteFile(filepath.Join(project, "go.mod"), []byte(module), 0o600); err != nil {
		t.Fatal(err)
	}

	if err = os.WriteFile(filepath.Join(project, "generated", "assets", "runtime_test.go"), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	command := exec.CommandContext(ctx, "go", "test", "-mod=mod", "-buildvcs=false", "./generated/...")
	command.Dir = project

	command.Env = append(os.Environ(), "GOWORK=off")
	if output, runErr := command.CombinedOutput(); runErr != nil {
		t.Fatalf("generated sprite catalog failed to compile or run: %v\n%s", runErr, output)
	}
}

func reviewPNG(t *testing.T, path string, width, height int, pixel color.NRGBA) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}

	pixels := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			pixels.SetNRGBA(x, y, pixel)
		}
	}

	//nolint:gosec // The path is constructed within this test's private temporary fixture.
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}

	if err = png.Encode(file, pixels); err != nil {
		_ = file.Close()

		t.Fatal(err)
	}

	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
}

const reviewRuntimeTest = `package assets

import (
    "encoding/binary"
    "math"
    "testing"

    "github.com/karty-game/ashura/pkg/draw2d"
)

func TestGeneratedImages(t *testing.T) {
    catalog, err := Load()
    if err != nil { t.Fatal(err) }
    images := []*draw2d.Image{catalog.UiSmallA, catalog.UiSmallB, catalog.UiBig, catalog.Standalone}
    expected := [][3]int{{2,2,128}, {2,2,96}, {6,6,255}, {3,2,255}}
    screen := draw2d.NewScreen()
    defer screen.Close()
    for index, image := range images {
        if image == nil || image.Width() != expected[index][0] || image.Height() != expected[index][1] {
            t.Fatalf("wrong named image dimensions at %d: %v", index, image)
        }
        screen.NewSprite(image).SetPosition(float32(index*10),0)
    }
    frame := screen.Frame(200,200)
    if len(frame.Batches) != 3 { t.Fatal("expected shared small images plus separate atlas/standalone pages",len(frame.Batches)) }
    if frame.Batches[0].First != 0 || frame.Batches[0].Count != 2 {
        t.Fatal("small regions did not share their prepared texture",frame.Batches[0])
    }
    if frame.Batches[0].Texture == frame.Batches[1].Texture || frame.Batches[0].Texture == frame.Batches[2].Texture || frame.Batches[1].Texture == frame.Batches[2].Texture {
        t.Fatal("separate prepared pages share a texture identity")
    }
    for _, batch := range frame.Batches {
        for slot := batch.First; slot < batch.First+batch.Count; slot++ {
            record := frame.Data[slot*80:]
            u := math.Float32frombits(binary.LittleEndian.Uint32(record[32:]))
            v := math.Float32frombits(binary.LittleEndian.Uint32(record[36:]))
            x,y := int(math.Round(float64(u*float32(batch.Texture.Width)))),int(math.Round(float64(v*float32(batch.Texture.Height))))
            offset := (y*batch.Texture.Width+x)*4
            if int(batch.Texture.Pixels[offset+3]) != expected[slot][2] {
                t.Fatal("generated region mapped to wrong source or lost straight alpha",slot)
            }
        }
    }
    // Payload admission fails before a malformed page can become a sprite.
    original := imagePage0
    defer func() { imagePage0 = original }()
    imagePage0 = original[:len(original)-1]
    if _, err := Load(); err == nil { t.Fatal("truncated embedded page was admitted") }
}
`
