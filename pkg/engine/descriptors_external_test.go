package engine_test

import (
	"testing"

	"github.com/karty-game/ashura/pkg/engine"
)

// These components deliberately implement only the public named contracts.
type externalNamedProjection struct{ settings engine.ProjectionData }

func (component *externalNamedProjection) ProjectionData() engine.ProjectionData {
	return component.settings
}

type externalNamedLight struct{ settings engine.LightData }

func (component *externalNamedLight) LightData() engine.LightData { return component.settings }

var (
	_ engine.ProjectionProvider = (*externalNamedProjection)(nil)
	_ engine.LightProvider      = (*externalNamedLight)(nil)
)

func TestApplicationComponentsUsePublicNamedContracts(t *testing.T) {
	t.Parallel()

	world := engine.NewWorld()

	camera := world.Add(engine.DefaultTransform(), externalNamedProjection{engine.ProjectionData{
		Kind: engine.OrthographicProjection, ViewHeight: 10, Near: .1, Far: 100,
	}})
	if err := world.SetActiveCamera(camera); err != nil {
		t.Fatal(err)
	}

	light := world.Add(externalNamedLight{engine.LightData{
		Kind: engine.PointLight, Position: engine.Vector3{Y: 2}, Color: engine.ColorWhite(), Range: 20,
		Shadow: engine.PointShadow{Enabled: true, Resolution: 512, Bias: 0},
	}})

	stored, ok := engine.Get[externalNamedLight](world, light)
	if !ok || stored.LightData().Kind != engine.PointLight || stored.LightData().Shadow.Bias != 0 {
		t.Fatal("public descriptor component was not retained with explicit settings")
	}

	stored.settings.Intensity = 2
	if stored.LightData().Intensity != 2 {
		t.Fatal("public descriptor component did not expose stored mutable fields")
	}
}
