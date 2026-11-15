package light_test

import (
	"testing"

	"github.com/karty-game/karty-webgpu/pkg/engine"
	"github.com/karty-game/karty-webgpu/pkg/light"
)

func TestLightDefaults(t *testing.T) {
	t.Parallel()

	hemisphere := light.NewHemisphere("sky", light.HemisphereOptions{})
	if hemisphere.Name != "sky" || hemisphere.Direction != engine.Vector3Up() || hemisphere.Intensity != 1 ||
		hemisphere.SkyColor != (engine.Color{R: .65, G: .75, B: 1, A: 1}) ||
		hemisphere.GroundColor != (engine.Color{R: .3, G: .27, B: .22, A: 1}) {
		t.Fatalf("unexpected hemisphere defaults: %+v", hemisphere)
	}

	directional := light.NewDirectional("sun", light.DirectionalOptions{})
	if directional.Name != "sun" || directional.Direction != (engine.Vector3{Y: -1}) ||
		directional.Color != engine.ColorWhite() || directional.Intensity != 1 {
		t.Fatalf("unexpected directional defaults: %+v", directional)
	}
}

func TestDirectionalAuthoredRayDirectionAndLiveDisable(t *testing.T) {
	t.Parallel()

	direction := engine.Vector3{X: 2, Y: -3, Z: 4}
	sun := light.NewDirectional("sun", light.DirectionalOptions{
		Direction: direction, Color: engine.ColorOrange(), Intensity: .5,
	})

	kind, rays, color, ground, intensity := sun.RuntimeLight()
	if kind != 2 || rays != [3]float32{2, -3, 4} || intensity != .5 || ground != ([3]float32{}) ||
		color != [3]float32{sun.Color.R, sun.Color.G, sun.Color.B} {
		t.Fatal("directional provider changed authored ray direction or light values")
	}

	sun.Intensity, sun.Color = 0, engine.Color{}

	kind, rays, color, ground, intensity = sun.RuntimeLight()
	if intensity != 0 || color != ([3]float32{}) || kind != 2 || rays != [3]float32{2, -3, 4} || ground != ([3]float32{}) {
		t.Fatal("live zero values restored constructor defaults instead of disabling light")
	}
}

func TestHemisphereComponentsAreIndependentAndMutable(t *testing.T) {
	t.Parallel()

	world := engine.NewWorld()
	shared := light.NewHemisphere("fill", light.HemisphereOptions{
		Direction: engine.Vector3{X: 2}, SkyColor: engine.ColorRed(), GroundColor: engine.ColorGreen(), Intensity: .5,
	})
	first, second := world.Add(shared), world.Add(shared)
	shared.Intensity = 10

	stored, found := engine.Get[light.Hemisphere](world, first)
	if !found {
		t.Fatal("hemisphere component missing")
	}

	stored.Direction, stored.Intensity = engine.Vector3Up(), 0
	stored.SkyColor, stored.GroundColor = engine.Color{}, engine.Color{}

	kind, direction, sky, ground, intensity := stored.RuntimeLight()
	if kind != 1 || direction != [3]float32{0, 1, 0} || sky != ([3]float32{}) || ground != ([3]float32{}) || intensity != 0 {
		t.Fatal("live hemisphere values restored constructor defaults")
	}

	other, found := engine.Get[light.Hemisphere](world, second)
	if !found || other.Intensity != .5 || other.Direction != (engine.Vector3{X: 2}) ||
		other.SkyColor != engine.ColorRed() || other.GroundColor != engine.ColorGreen() {
		t.Fatal("mutating one hemisphere changed another component")
	}
}
