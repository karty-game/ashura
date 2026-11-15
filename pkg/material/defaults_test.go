package material_test

import (
	"testing"

	"github.com/karty-game/karty-webgpu/pkg/engine"
	"github.com/karty-game/karty-webgpu/pkg/material"
)

func TestExplicitStandardDefaultsPreserveDisabledFeatures(t *testing.T) {
	t.Parallel()

	value := material.DefaultStandard()
	if value != material.Standard(material.Options{}) {
		t.Fatal("explicit defaults differ from the compatibility constructor")
	}

	value.NormalStrength, value.AOStrength = 0, 0
	world := engine.NewWorld()
	entity := world.Add(value)

	stored, ok := engine.Get[material.Material](world, entity)
	if !ok {
		t.Fatal("material was not attached")
	}

	albedo, normal, scale, normalStrength := stored.RuntimeTextures()
	_, aoStrength := stored.RuntimeOcclusion()

	if albedo != nil || normal != nil || scale != (engine.Vector2{X: 1, Y: 1}) || normalStrength != 0 || aoStrength != 0 {
		t.Fatal("explicit disabled strengths acquired defaults during attachment")
	}

	if next := material.DefaultStandard(); next.NormalStrength != 1 || next.AOStrength != 1 {
		t.Fatal("changing one default value mutated future defaults")
	}
}

func TestExplicitSourceDefaultsPreserveDirectOnlyLighting(t *testing.T) {
	t.Parallel()

	value := material.DefaultSource()
	if value != material.Source(material.SourceOptions{}) {
		t.Fatal("explicit defaults differ from the compatibility constructor")
	}

	value.AmbientColors, value.SSBumpStrength = [3]engine.Color{}, 0
	world := engine.NewWorld()
	entity := world.Add(value)

	stored, ok := engine.Get[material.SourceMaterial](world, entity)
	if !ok {
		t.Fatal("material was not attached")
	}

	ambient, _, strength := stored.RuntimeSource()
	if ambient != ([3]engine.Color{}) || strength != 0 {
		t.Fatal("direct-only/disabled-map settings acquired constructor defaults")
	}
}
