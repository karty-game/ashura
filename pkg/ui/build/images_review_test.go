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
	uibuild "github.com/karty-game/ashura/pkg/ui/build"
)

func TestUIImagesReviewGeneratedCatalogRunsWithMultiplePagesAndStandaloneImage(t *testing.T) {
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
				uibuild.Images(),
			},
		}},
	}
	if _, err := assetbuild.Build(t.Context(), config, assetbuild.Options{Project: project, Identity: "ui-images-review@1"}); err != nil {
		t.Fatal(err)
	}

	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}

	module := fmt.Sprintf(
		"module github.com/karty-game/ashura/internal/ui-recipe-fixture\n\ngo 1.27.0\n\nrequire github.com/karty-game/ashura v0.0.0\n\nreplace github.com/karty-game/ashura => %q\n",
		root,
	)
	if err = os.WriteFile(filepath.Join(project, "go.mod"), []byte(module), 0o600); err != nil {
		t.Fatal(err)
	}

	if err = os.WriteFile(filepath.Join(project, "generated", "assets", "runtime_test.go"), []byte(reviewRuntimeTest), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	command := exec.CommandContext(ctx, "go", "test", "-mod=mod", "-buildvcs=false", "./generated/...")
	command.Dir = project

	command.Env = append(os.Environ(), "GOWORK=off")
	if output, runErr := command.CombinedOutput(); runErr != nil {
		t.Fatalf("generated UI catalog failed to compile or run: %v\n%s", runErr, output)
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

const reviewRuntimeTest = `package assets_test

import (
    "encoding/binary"
    "math"
    "testing"
    "time"

    "github.com/karty-game/ashura/pkg/ui"
    "github.com/karty-game/ashura/pkg/ui/fonts/defaultfont"
    assets "github.com/karty-game/ashura/internal/ui-recipe-fixture/generated/assets"
)

func TestGeneratedImages(t *testing.T) {
    catalog, err := assets.Load()
    if err != nil { t.Fatal(err) }
    images := []*ui.ImageAsset{catalog.UiSmallA, catalog.UiSmallB, catalog.UiBig, catalog.Standalone}
    expected := [][3]int{{2,2,128}, {2,2,96}, {6,6,255}, {3,2,255}}
    children := make([]*ui.Node, 0, len(images))
    for index, asset := range images {
        if asset == nil || asset.Width() != expected[index][0] || asset.Height() != expected[index][1] {
            t.Fatalf("wrong named image dimensions at %d: %v", index, asset)
        }
        node, nodeErr := ui.NewImageFromAsset(asset)
        if nodeErr != nil { t.Fatal(nodeErr) }
        children = append(children, node.WithStyle(ui.Style{Width:16, Height:16}))
    }
    font, err := defaultfont.Load()
    if err != nil { t.Fatal(err) }
    view, err := ui.New(ui.Column(children...), font)
    if err != nil { t.Fatal(err) }
    if err = view.Resize(200,200); err != nil { t.Fatal(err) }
    frame, err := view.Frame(time.Second)
    if err != nil { t.Fatal(err) }
    active := []int{}
    for index, batch := range frame.Batches {
        if batch.Count > 0 { active = append(active, index) }
    }
    if len(active) != 4 { t.Fatal("wrong number of image batches", active) }
    a,b,c,d := frame.Batches[active[0]],frame.Batches[active[1]],frame.Batches[active[2]],frame.Batches[active[3]]
    if a.Texture != b.Texture || a.Texture == c.Texture || a.Texture == d.Texture || c.Texture == d.Texture {
        t.Fatal("generated catalog did not share small regions or distinguish atlas/standalone pages")
    }
    for index, batchIndex := range active {
        batch := frame.Batches[batchIndex]
        record := frame.Data[batch.First*96:]
        u := math.Float32frombits(binary.LittleEndian.Uint32(record[64:]))
        v := math.Float32frombits(binary.LittleEndian.Uint32(record[68:]))
        x,y := int(math.Round(float64(u*float32(batch.Texture.Width)))),int(math.Round(float64(v*float32(batch.Texture.Height))))
        offset := (y*batch.Texture.Width+x)*4
        if int(batch.Texture.Pixels[offset+3]) != expected[index][2] {
            t.Fatal("generated region mapped to wrong source or lost straight alpha", index)
        }
    }
}
`
