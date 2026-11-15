package reflection_test

import (
	"testing"

	"github.com/karty-game/karty-webgpu/pkg/engine"
	"github.com/karty-game/karty-webgpu/pkg/reflection"
)

func TestDefaultProbeAllowsExplicitDisabledStrength(t *testing.T) {
	t.Parallel()

	probe := reflection.DefaultProbe("room", nil)
	if probe != reflection.NewProbe("room", nil, reflection.ProbeOptions{}) {
		t.Fatal("explicit defaults differ from compatibility constructor")
	}

	probe.Strength = 0
	probe.HalfExtents = engine.Vector3One()
	world := engine.NewWorld()
	entity := world.Add(probe)
	stored, _ := engine.Get[reflection.Probe](world, entity)

	_, _, strength := stored.RuntimeReflectionProbe()
	if strength != 0 {
		t.Fatal("disabled probe acquired a constructor default")
	}
}
