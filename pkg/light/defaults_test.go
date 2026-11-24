package light_test

import (
	"testing"

	"github.com/karty-game/ashura/pkg/engine"
	"github.com/karty-game/ashura/pkg/light"
)

func TestExplicitLightDefaultsStartDisabledWithoutNormalization(t *testing.T) {
	t.Parallel()

	point := light.DefaultPoint("point")
	spot := light.DefaultSpot("spot")
	directional := light.DefaultDirectional("sun")
	hemisphere := light.DefaultHemisphere("sky")

	if point != light.NewPoint("point", light.PointOptions{}) ||
		directional != light.NewDirectional("sun", light.DirectionalOptions{}) ||
		hemisphere != light.NewHemisphere("sky", light.HemisphereOptions{}) {
		t.Fatal("explicit defaults differ from compatibility constructors")
	}

	point.Intensity, point.ShadowBias = 0, 0
	spot.Intensity, spot.CastShadows = 0, true
	directional.Intensity, hemisphere.Intensity = 0, 0
	world := engine.NewWorld()
	p := world.Add(point)
	s := world.Add(spot)
	d := world.Add(directional)
	h := world.Add(hemisphere)
	storedPoint, _ := engine.Get[light.Point](world, p)
	storedSpot, _ := engine.Get[light.Spot](world, s)
	storedDirectional, _ := engine.Get[light.Directional](world, d)
	storedHemisphere, _ := engine.Get[light.Hemisphere](world, h)

	if data := storedPoint.LightData(); data.Intensity != 0 || data.Shadow.Bias != 0 {
		t.Fatal("disabled point light or unbiased shadow acquired defaults")
	}

	if data := storedSpot.LightData(); data.Intensity != 0 || !data.Shadow.Enabled {
		t.Fatal("disabled spotlight lost its intensity or shadow declaration")
	}

	if storedDirectional.LightData().Intensity != 0 || storedHemisphere.LightData().Intensity != 0 {
		t.Fatal("disabled lighting acquired defaults")
	}
}
