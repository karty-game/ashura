package engine_test

import (
	"testing"

	"github.com/karty-game/ashura/pkg/engine"
	"github.com/karty-game/ashura/pkg/material"
)

func TestPublicMaterialReplacementAndExplicitRemoval(t *testing.T) {
	t.Parallel()

	world := engine.NewWorld()
	source := material.Source(material.SourceOptions{Color: engine.ColorRed()})
	standard := material.Standard(material.Options{Color: engine.ColorGreen()})

	entity := world.Add(source)
	if err := world.Set(entity, standard); err != nil {
		t.Fatal(err)
	}

	if _, present := engine.Get[material.SourceMaterial](world, entity); !present {
		t.Fatal("assigning Standard silently deleted the retained Source component")
	}

	if stored, present := engine.Get[material.Material](world, entity); !present || stored.Color != standard.Color {
		t.Fatal("public Standard assignment did not retain its typed component value")
	}

	source.Color = engine.ColorBlue()
	if err := world.Set(entity, source); err != nil {
		t.Fatal(err)
	}

	if !engine.RemoveComponent[material.SourceMaterial](world, entity) {
		t.Fatal("explicit public Source removal failed")
	}

	if _, present := engine.Get[material.SourceMaterial](world, entity); present {
		t.Fatal("removed Source component remained available")
	}

	count := 0

	for result, surface := range engine.Query[material.Material](world) {
		if result != entity || surface.Color != standard.Color {
			t.Fatal("retained Standard component/query pointer changed")
		}

		count++
	}

	if count != 1 {
		t.Fatalf("retained Standard query count=%d", count)
	}
}
