//nolint:mnd // The sample's colors, lights and layout are authored in world units.
package main

import (
	"github.com/karty-game/ashura/pkg/camera"
	"github.com/karty-game/ashura/pkg/engine"
	"github.com/karty-game/ashura/pkg/light"
	"github.com/karty-game/ashura/pkg/material"
	"github.com/karty-game/ashura/pkg/mesh"
	"github.com/karty-game/ashura/samples/internal/sample"
)

const (
	wallY        = 1.08
	loweredWallY = -60
	wallOffsetX  = .05
	plumeZ       = 1.4
)

func createScene(app *engine.App) (*engine.World, error) {
	world := engine.NewWorld()
	projection, controls := camera.Orbit()
	projection.FieldOfViewDegrees = 45
	controls.Target, controls.Distance = engine.Vector3{Y: 1.42, Z: -.2}, 8.2
	pose := engine.DefaultTransform()
	pose.Rotation.X, pose.Rotation.Y = -.17, .035

	view := world.Add(pose, projection, controls)
	if err := world.SetActiveCamera(view); err != nil {
		return nil, err
	}

	sample.FitCamera(app, world, view)

	sky := light.DefaultHemisphere("sky")
	sky.Intensity = .68
	world.Add(sky)

	key := light.DefaultDirectional("key")
	key.Direction, key.Intensity = engine.Vector3{X: -.8, Y: -1, Z: -.65}, 1.25
	world.Add(key)

	wall := addStage(world)

	if err := addGallery(app, wall); err != nil {
		return nil, err
	}

	return world, nil
}

func addStage(world *engine.World) *engine.Transform {
	floor := material.DefaultStandard()
	floor.Color = engine.Color{R: .059, G: .073, B: .095, A: 1}
	world.Add(mesh.NewBox("stage", mesh.BoxOptions{Width: 8.2, Height: .28, Depth: 5.4}), engine.Transform{
		Position: engine.Vector3{Y: -.14}, Scale: engine.Vector3One(),
	}, floor)

	concrete := material.DefaultStandard()
	concrete.Color = engine.Color{R: .14, G: .19, B: .25, A: 1}
	wall := world.Add(mesh.NewBox("blast wall", mesh.BoxOptions{Width: 2.15, Height: 2.16, Depth: .58}), engine.Transform{
		Position: engine.Vector3{X: galleryCenterX + wallOffsetX, Y: loweredWallY, Z: .62}, Scale: engine.Vector3One(),
	}, concrete)

	metal := material.DefaultStandard()
	metal.Color = engine.Color{R: .09, G: .15, B: .2, A: 1}
	world.Add(mesh.NewBox("plume nozzle", mesh.BoxOptions{Width: .94, Height: .32, Depth: .94}), engine.Transform{
		Position: engine.Vector3{X: 2, Y: .16, Z: plumeZ}, Scale: engine.Vector3One(),
	}, metal)

	return world.Transform(wall)
}
