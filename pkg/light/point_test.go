package light_test

import (
	"testing"

	"github.com/karty-game/ashura/pkg/engine"
	"github.com/karty-game/ashura/pkg/light"
)

func TestPointDefaultsAndExplicitShadowSettings(t *testing.T) {
	t.Parallel()

	point := light.NewPoint("torch", light.PointOptions{Position: engine.Vector3{X: 1}})
	if point.Intensity != 1 || point.Range != 20 || point.ShadowResolution != 512 || point.ShadowBias != .015 || point.CastShadows {
		t.Fatalf("point defaults: %+v", point)
	}

	explicit := light.NewPoint(
		"torch",
		light.PointOptions{Intensity: 3, Range: 7, CastShadows: true, ShadowResolution: 256, ShadowBias: .02},
	)

	kind, position, color, ground, intensity := explicit.RuntimeLight()
	if kind == 0 || position != [3]float32{} || color != [3]float32{1, 1, 1} {
		t.Fatal("point source fields are invalid")
	}

	if ground[0] != 7 || ground[1] != 1 || intensity != 3 {
		t.Fatalf("point runtime values: ground=%v intensity=%v", ground, intensity)
	}

	explicit.Intensity = 0

	kind, position, color, ground, intensity = explicit.RuntimeLight()
	if kind == 0 || position != [3]float32{} || color != [3]float32{1, 1, 1} {
		t.Fatal("disabled source lost source fields")
	}

	if ground[1] != 0 || intensity != 0 {
		t.Fatal("disabled light keeps an active shadow flag")
	}
}
