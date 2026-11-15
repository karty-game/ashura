package main

import (
	"github.com/karty-game/karty-webgpu/pkg/camera"
	"github.com/karty-game/karty-webgpu/pkg/engine"
	"github.com/karty-game/karty-webgpu/pkg/light"
	"github.com/karty-game/karty-webgpu/pkg/mesh"
	"github.com/karty-game/karty-webgpu/samples/internal/sample"
)

const (
	groundSize     = 12
	groundTiles    = 6
	cubeSize       = 1.8
	cubeHalfHeight = cubeSize / 2
	cubeYaw        = -.3
	cubeX          = -1.3
	sphereX        = 1.3
	sphereZ        = -.6
	sphereTiles    = 3
	cameraX        = 4
	cameraY        = 3.2
	cameraZ        = 7
	cameraYaw      = .5
	cameraPitch    = -.29
	fillIntensity  = .18
	sunIntensity   = 1.2
	sunX           = -.7
	sunY           = -.45
	sunZ           = -.6
	stoneRoughness = .48
	brickRoughness = .65
)

func createScene(app *engine.App) (*engine.World, error) {
	stone, brick, err := loadMaterials(app)
	if err != nil {
		return nil, err
	}

	world := engine.NewWorld()
	projection, controls := camera.FirstPerson()

	view := world.Add(engine.Transform{
		Position: engine.Vector3{X: cameraX, Y: cameraY, Z: cameraZ},
		Rotation: engine.Vector3{X: cameraPitch, Y: cameraYaw}, Scale: engine.Vector3One(),
	}, projection, controls)
	if err := world.SetActiveCamera(view); err != nil {
		return nil, err
	}

	sample.FitCamera(app, world, view)

	fill := light.DefaultHemisphere("fill")
	fill.Intensity = fillIntensity
	sun := light.DefaultDirectional("raking light")
	sun.Direction, sun.Intensity = engine.Vector3{X: sunX, Y: sunY, Z: sunZ}, sunIntensity

	world.Add(fill)
	world.Add(sun)

	stone.setUVScale(engine.Vector2{X: groundTiles, Y: groundTiles})
	ground := world.Add(mesh.NewPlane("stone ground", mesh.PlaneOptions{Width: groundSize, Height: groundSize}),
		engine.DefaultTransform(), stone.standard)
	cube := world.Add(
		mesh.NewBox("brick cube", mesh.BoxOptions{Width: cubeSize, Height: cubeSize, Depth: cubeSize}),
		engine.Transform{
			Position: engine.Vector3{X: cubeX, Y: cubeHalfHeight},
			Rotation: engine.Vector3{Y: cubeYaw},
			Scale:    engine.Vector3One(),
		},
		brick.standard,
	)
	sphereMaterial := brick
	sphereMaterial.setUVScale(engine.Vector2{X: sphereTiles, Y: sphereTiles})
	sphere := world.Add(
		mesh.NewSphere("brick sphere", mesh.SphereOptions{Diameter: cubeSize}),
		engine.Transform{
			Position: engine.Vector3{X: sphereX, Y: cubeHalfHeight, Z: sphereZ},
			Scale:    engine.Vector3One(),
		},
		sphereMaterial.standard,
	)

	demo := &materialDemo{world: world, surfaces: []materialSurface{
		{entity: ground, pair: stone}, {entity: cube, pair: brick}, {entity: sphere, pair: sphereMaterial},
	}}
	if err := demo.setupUI(app); err != nil {
		return nil, err
	}

	demo.attachControls(app)

	return world, nil
}
