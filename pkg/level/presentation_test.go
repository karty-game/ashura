//go:build !js

package level

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/karty-game/karty-webgpu/internal/leveldata"
	"github.com/karty-game/karty-webgpu/internal/modeldata"
	"github.com/karty-game/karty-webgpu/pkg/engine"
	"github.com/karty-game/karty-webgpu/pkg/material"
)

func presentationChunk(t *testing.T, root string, counts ...int) Chunk {
	t.Helper()
	description := fixtureChunk(t, root)
	original := description.Pages[0]

	//nolint:gosec // fixtureChunk creates this checksummed filename beneath the test-owned temporary root.
	data, err := os.ReadFile(filepath.Join(root, original.Geometry.Path))
	if err != nil {
		t.Fatal(err)
	}

	source, err := modeldata.Decode(data)
	if err != nil {
		t.Fatal(err)
	}

	description.Pages = nil

	for pageIndex, count := range counts {
		page := original
		page.Materials = nil
		document := modeldata.Document{Source: fmt.Sprintf("presentation%d", pageIndex)}

		streams := make([][]byte, count)
		for index := range count {
			part := source.Parts[0]
			part.Name = fmt.Sprintf("part%d", index)
			document.Parts = append(document.Parts, part)
			options := original.Materials[0]
			options.Name = part.Name
			options.Color[0] = float32(index+1) / float32(count+1)
			page.Materials = append(page.Materials, options)
			streams[index] = make([]byte, 24)
		}

		encoded, err := modeldata.Encode(document)
		if err != nil {
			t.Fatal(err)
		}

		uvData, err := leveldata.EncodeUV(sha256.Sum256(encoded), streams)
		if err != nil {
			t.Fatal(err)
		}

		page.Geometry = payload(t, root, encoded)
		page.UV = payload(t, root, uvData)
		description.Pages = append(description.Pages, page)
	}

	return description
}

func loadPresentationChunk(t *testing.T, counts ...int) (*engine.App, *Loader, *LoadedChunk) {
	t.Helper()
	root := t.TempDir()
	description := presentationChunk(t, root, counts...)

	prepared, err := ReadChunk(t.Context(), root, description)
	if err != nil {
		t.Fatal(err)
	}

	app := engine.NewApp(engine.AppConfig{})

	t.Cleanup(func() { _ = app.Close() })

	loader := NewLoader(app)

	chunk, err := loader.Load(prepared)
	if err != nil {
		t.Fatal(err)
	}

	return app, loader, chunk
}

func TestPresentationSelectsBoundedPreparedRangesAndMatchesSynchronousInstance(t *testing.T) {
	t.Parallel()
	_, _, chunk := loadPresentationChunk(t, 19, 3, 10)
	world := engine.NewWorld()

	instance, err := chunk.BeginPresentation(world)
	if err != nil {
		t.Fatal(err)
	}

	if world.Len() != 0 {
		t.Fatal("BeginPresentation added draw records")
	}

	if err = chunk.Unload(); err == nil {
		t.Fatal("pending presentation did not retain chunk lifetime")
	}

	for step, expected := range []int{8, 8, 3, 3, 8, 2} {
		before := world.Len()

		done, err := instance.PresentNext()
		if err != nil {
			t.Fatal(err)
		}

		if world.Len()-before != expected || expected > PresentationRangesPerStep {
			t.Fatal("presentation crossed a page or exceeded the prebuilt-range bound")
		}

		if done != (step == 5) {
			t.Fatal("presentation completion does not match final prepared range")
		}
	}

	if done, err := instance.PresentNext(); !done || err != nil || world.Len() != 32 {
		t.Fatalf("completed presentation not idempotent: %v", err)
	}

	expectedColors := [][4]float32{}

	for _, page := range chunk.description.Pages {
		for _, source := range page.Materials {
			expectedColors = append(expectedColors, source.Color)
		}
	}

	checkPresentationColors(t, world, expectedColors)

	synchronous := engine.NewWorld()

	complete, err := chunk.Instantiate(synchronous)
	if err != nil {
		t.Fatal(err)
	}

	checkPresentationColors(t, synchronous, expectedColors)
	instance.Remove()
	complete.Remove()

	if world.Len() != 0 || synchronous.Len() != 0 {
		t.Fatal("range presentation removal retained entities")
	}

	if err = chunk.Unload(); err != nil {
		t.Fatal(err)
	}
}

func checkPresentationColors(t *testing.T, world *engine.World, expected [][4]float32) {
	t.Helper()

	index := 0

	world.Each(func(entity engine.Entity) {
		source, ok := engine.Get[material.SourceMaterial](world, entity)
		if !ok || index >= len(expected) ||
			source.Color != (engine.Color{R: expected[index][0], G: expected[index][1], B: expected[index][2], A: expected[index][3]}) {
			t.Fatal("prepared range/material order changed")
		}

		if _, standard := engine.Get[material.Material](world, entity); standard {
			t.Fatal("temporary Standard material survived Source range presentation")
		}

		index++
	})

	if index != len(expected) {
		t.Fatal("presentation omitted prepared ranges")
	}
}

func TestPartialPresentationRemoveAndAppCloseReleaseOwnership(t *testing.T) {
	t.Parallel()
	app, loader, chunk := loadPresentationChunk(t, 17)
	world := engine.NewWorld()

	empty, err := chunk.BeginPresentation(world)
	if err != nil {
		t.Fatal(err)
	}

	empty.Remove()
	empty.Remove()

	if chunk.inUse() || world.Len() != 0 {
		t.Fatal("empty presentation cancellation retained ownership")
	}

	instance, err := chunk.BeginPresentation(world)
	if err != nil {
		t.Fatal(err)
	}

	if done, err := instance.PresentNext(); done || err != nil || world.Len() != 8 {
		t.Fatalf("partial presentation: %v", err)
	}

	if err = chunk.Unload(); err == nil {
		t.Fatal("partial visible presentation unloaded")
	}

	if err = app.Close(); err != nil {
		t.Fatal(err)
	}

	if world.Len() != 0 || !chunk.closed || !loader.closed || instance.chunk != nil {
		t.Fatal("App close retained partial presentation")
	}

	if _, err = instance.PresentNext(); err == nil {
		t.Fatal("closed presentation continued")
	}
}

func TestPresentationFailureRollsBackEarlierVisibleRanges(t *testing.T) {
	t.Parallel()
	_, _, chunk := loadPresentationChunk(t, 8, 1)
	world := engine.NewWorld()

	instance, err := chunk.BeginPresentation(world)
	if err != nil {
		t.Fatal(err)
	}

	if done, err := instance.PresentNext(); done || err != nil || world.Len() != 8 {
		t.Fatalf("first prebuilt page: %v", err)
	}

	if err = chunk.pages[1].model.Unload(); err != nil {
		t.Fatal(err)
	}

	if done, err := instance.PresentNext(); !done || err == nil || world.Len() != 0 || len(chunk.instances) != 0 {
		t.Fatal("failed presentation retained earlier visible ranges")
	}

	if err = chunk.Unload(); err != nil {
		t.Fatal(err)
	}
}
