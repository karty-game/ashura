package main

import (
	"math"
	"testing"

	"github.com/karty-game/ashura/pkg/engine"
	"github.com/karty-game/ashura/samples/luminous/internal/courtyard"
)

func TestAnimatedLightsClearPillars(t *testing.T) {
	t.Parallel()

	world := engine.NewWorld()
	rig := addLights(world)

	const layouts, steps, stepSeconds = 3, 600, .1

	for mode := range layouts {
		rig.mode = mode
		for step := range steps {
			rig.elapsed = float64(step) * stepSeconds
			if err := rig.update(); err != nil {
				t.Fatal(err)
			}

			checkPillarClearance(t, rig.point.Name, rig.point.Position, mode, rig.elapsed)

			for _, point := range rig.accents {
				if point.Intensity > 0 {
					checkPillarClearance(t, point.Name, point.Position, mode, rig.elapsed)
				}
			}
		}
	}
}

func checkPillarClearance(t *testing.T, name string, position engine.Vector3, mode int, seconds float64) {
	t.Helper()

	for _, pillar := range courtyard.Pillars() {
		if position.Y > 0 && position.Y < pillar.Height &&
			math.Abs(float64(position.X-pillar.X)) < float64(pillar.Width)/2 &&
			math.Abs(float64(position.Z-pillar.Z)) < float64(pillar.Depth)/2 {
			t.Fatalf("%s entered pillar at (%v, %v) in layout %d at %.2fs", name, pillar.X, pillar.Z, mode, seconds)
		}
	}
}

func TestLightLayoutsActivateTheirFullBudgets(t *testing.T) {
	t.Parallel()

	rig := addLights(engine.NewWorld())

	for mode, count := range []int{240, 96, 240} {
		rig.mode = mode
		if err := rig.update(); err != nil {
			t.Fatal(err)
		}

		active := 0

		for _, accent := range rig.accents {
			if accent.Intensity > 0 {
				active++
			}
		}

		if active+heroLightCount != count {
			t.Fatalf("layout %d has %d active lights, want %d", mode, active+heroLightCount, count)
		}
	}
}
