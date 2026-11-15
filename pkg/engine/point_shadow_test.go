package engine

import (
	"testing"

	"github.com/karty-game/karty-webgpu/internal/render"
)

type startupPoint struct {
	intensity  float32
	cast       bool
	resolution uint32
}

func (point startupPoint) RuntimeLight() (uint32, [3]float32, [3]float32, [3]float32, float32) {
	return render.PointLightKind, [3]float32{1, 2, 3}, [3]float32{1, 1, 1}, [3]float32{20, 0, 0}, point.intensity
}

func (point startupPoint) RuntimePointShadow() (bool, uint32, float32) {
	return point.cast, point.resolution, .015
}

func TestPointShadowStartupIncludesDisabledDeclaredSource(t *testing.T) {
	t.Parallel()

	world := NewWorld()
	world.Add(startupPoint{intensity: 0, cast: true, resolution: 512})

	resolution, err := pointShadowResolution(world)
	if err != nil || resolution != 512 {
		t.Fatalf("disabled shadow startup = %d,%v", resolution, err)
	}
}

func TestPointShadowStartupRejectsMultipleDeclaredSources(t *testing.T) {
	t.Parallel()

	world := NewWorld()
	world.Add(startupPoint{intensity: 0, cast: true, resolution: 512})
	world.Add(startupPoint{intensity: 0, cast: true, resolution: 512})

	if _, err := pointShadowResolution(world); err == nil {
		t.Fatal("multiple disabled shadow declarations bypass startup admission")
	}
}

func TestPointShadowStartupWithoutShadowsNeedsNoDepthPipelines(t *testing.T) {
	t.Parallel()

	world := NewWorld()
	world.Add(startupPoint{intensity: 1, cast: false, resolution: 512})

	resolution, err := pointShadowResolution(world)
	if err != nil || resolution != 0 {
		t.Fatalf("unshadowed startup = %d,%v", resolution, err)
	}
}
