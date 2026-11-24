//go:build !js

package level

import (
	"testing"

	"github.com/karty-game/ashura/internal/leveldata"
	"github.com/karty-game/ashura/pkg/engine"
	"github.com/karty-game/ashura/pkg/material"
	"github.com/karty-game/ashura/pkg/texture"
)

func TestRGBChunkUsesOneCoefficientAndCalibratedStandardMaterial(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	chunk := fixtureChunk(t, root)
	page := &chunk.Pages[0]
	page.Representation = leveldata.RGBDiffuse
	page.Maps[1], page.Maps[2] = Reference{}, Reference{}
	page.Materials[0].SSBump = nil

	prepared, err := ReadChunk(t.Context(), root, chunk)
	if err != nil {
		t.Fatal(err)
	}

	if len(prepared.imageRefs) != 2 {
		t.Fatal("RGB fetched extra coefficients or detail maps", len(prepared.imageRefs))
	}

	app := engine.NewApp(engine.AppConfig{})

	t.Cleanup(func() { _ = app.Close() })

	loaded, err := NewLoader(app).Load(prepared)
	if err != nil {
		t.Fatal(err)
	}

	if len(loaded.pages[0].standard) != 1 || len(loaded.pages[0].materials) != 0 {
		t.Fatal("RGB used the directional material")
	}

	world := engine.NewWorld()

	instance, err := loaded.Instantiate(world)
	if err != nil {
		t.Fatal(err)
	}

	entity := instance.models[0].Entities()[0]

	component, ok := engine.Get[material.Material](world, entity)
	if !ok || component.RGBLightmap == nil || component.RGBLightmap.Encoding != texture.RNMRGBE ||
		component.RGBLightmap.Scale != page.Scale {
		t.Fatal(component, ok)
	}
}
