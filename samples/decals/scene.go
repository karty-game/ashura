package main

import (
	"math"

	"github.com/karty-game/ashura/pkg/camera"
	"github.com/karty-game/ashura/pkg/collision"
	"github.com/karty-game/ashura/pkg/engine"
	"github.com/karty-game/ashura/pkg/light"
	"github.com/karty-game/ashura/samples/decals/generated/assets"
	"github.com/karty-game/ashura/samples/decals/generated/assets/colliders"
	"github.com/karty-game/ashura/samples/internal/sample"
)

const fieldOfView = 41

//nolint:mnd // Camera placement and radiance are authored for the yard.
func createScene(app *engine.App) (*engine.World, error) {
	atlas, err := assets.FoxMuralAlbedoRegion().Asset.Load(app)
	if err != nil {
		return nil, err
	}

	world := engine.NewWorld()
	projection, orbit := camera.Orbit()
	projection.FieldOfViewDegrees = fieldOfView
	orbit.Target = engine.Vector3{Y: 2.05, Z: -1.10}
	orbit.RotateButton = engine.MouseButtonRight
	orbit.DisableTouchRotate = true
	orbit.Distance = 24.4
	orbit.MinDistance, orbit.MaxDistance = 7, 38

	transform := engine.DefaultTransform()
	transform.Rotation = engine.Vector3{X: -.205, Y: .11}

	cameraEntity := world.Add(transform, projection, orbit)
	if err := world.SetActiveCamera(cameraEntity); err != nil {
		return nil, err
	}

	sample.FitCamera(app, world, cameraEntity)
	addLights(world)

	if err := buildYard(app, world); err != nil {
		return nil, err
	}

	var hitTest collision.World
	hitTest.AddMesh(colliders.Yard(), yardCollisionLayer)

	if err := attachSprayer(app, world, cameraEntity, &hitTest, atlas); err != nil {
		return nil, err
	}

	return world, nil
}

//nolint:mnd // Warm key light, cool fill, and local fixtures are authored lighting.
func addLights(world *engine.World) {
	fill := light.DefaultHemisphere("blue dusk")
	fill.SkyColor = engine.Color{R: .64, G: .83, B: 1, A: 1}
	fill.GroundColor = engine.Color{R: .29, G: .22, B: .30, A: 1}
	fill.Intensity = .65
	world.Add(fill)

	key := light.DefaultDirectional("overcast break")
	key.Color = engine.Color{R: 1, G: .82, B: .69, A: 1}
	key.Direction = engine.Vector3{X: -.46, Y: -.62, Z: -.78}
	key.Intensity = .91
	world.Add(key)

	for _, x := range []float32{-9.65, 2.2, 9.8} {
		lamp := light.DefaultPoint("sodium yard lamp")
		lamp.Position = engine.Vector3{X: x, Y: 4.52, Z: -3.59}
		lamp.Color = engine.Color{R: 1, G: .62, B: .31, A: 1}
		lamp.Intensity, lamp.Range = .7, 3.6
		world.Add(lamp)
	}
}

// pointerRay inverts the camera's perspective and its pitch/yaw rotation.
func pointerRay(u, v, aspect, fov float32, rotation engine.Vector3) engine.Vector3 {
	halfHeight := math.Tan(float64(fov) * math.Pi / 360)
	viewX, viewY := float64((2*u-1)*aspect)*halfHeight, float64(1-2*v)*halfHeight
	sinPitch, cosPitch := math.Sincos(float64(rotation.X))
	sinYaw, cosYaw := math.Sincos(float64(rotation.Y))
	depth := viewY*sinPitch - cosPitch

	return engine.Vector3{
		X: float32(viewX*cosYaw + depth*sinYaw),
		Y: float32(viewY*cosPitch + sinPitch),
		Z: float32(-viewX*sinYaw + depth*cosYaw),
	}
}
