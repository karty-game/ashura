package main

import (
	"time"

	"github.com/karty-game/ashura/pkg/camera"
	"github.com/karty-game/ashura/pkg/engine"
	"github.com/karty-game/ashura/pkg/light"
	"github.com/karty-game/ashura/pkg/mesh"
	"github.com/karty-game/ashura/samples/internal/sample"
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
	stone, brick := defaultMaterialPair(stoneRoughness), defaultMaterialPair(brickRoughness)

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
		engine.DefaultTransform())
	cube := world.Add(
		mesh.NewBox("brick cube", mesh.BoxOptions{Width: cubeSize, Height: cubeSize, Depth: cubeSize}),
		engine.Transform{
			Position: engine.Vector3{X: cubeX, Y: cubeHalfHeight},
			Rotation: engine.Vector3{Y: cubeYaw},
			Scale:    engine.Vector3One(),
		},
	)
	sphereMaterial := brick
	sphereMaterial.setUVScale(engine.Vector2{X: sphereTiles, Y: sphereTiles})
	sphere := world.Add(
		mesh.NewSphere("brick sphere", mesh.SphereOptions{Diameter: cubeSize}),
		engine.Transform{
			Position: engine.Vector3{X: sphereX, Y: cubeHalfHeight, Z: sphereZ},
			Scale:    engine.Vector3One(),
		},
	)

	demo := &materialDemo{world: world, surfaces: []materialSurface{
		{entity: ground, pair: stone}, {entity: cube, pair: brick}, {entity: sphere, pair: sphereMaterial},
	}}
	if err := demo.setupUI(app); err != nil {
		return nil, err
	}

	demo.prepareMaterials(app)
	demo.attachControls(app)

	return world, nil
}

func (demo *materialDemo) prepareMaterials(app *engine.App) {
	var stop func()

	stop = app.OnUpdateError(func(time.Duration) error {
		stop()

		format, err := materialFormat(app.SupportsTextureFormat)
		if err != nil {
			return err
		}

		stone, brick, err := loadMaterialsForFormat(app, format)
		if err != nil {
			return err
		}

		for index, pair := range []materialPair{stone, brick, brick} {
			pair.setUVScale(demo.surfaces[index].pair.standard.UVScale)
			demo.surfaces[index].pair = pair
		}

		return demo.apply(demo.useSource)
	})
}
