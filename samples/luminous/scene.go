package main

import (
	"github.com/karty-game/ashura/pkg/camera"
	"github.com/karty-game/ashura/pkg/engine"
	"github.com/karty-game/ashura/pkg/material"
	"github.com/karty-game/ashura/pkg/mesh"
	"github.com/karty-game/ashura/samples/internal/sample"
	"github.com/karty-game/ashura/samples/luminous/internal/courtyard"
)

//nolint:mnd // Orbit framing is authored for the courtyard.
func createScene(app *engine.App) (*engine.World, error) {
	world := engine.NewWorld()
	projection, orbit := camera.Orbit()
	projection.Far, projection.FieldOfViewDegrees = 90, 55
	orbit.Distance, orbit.MinDistance, orbit.MaxDistance = 29, 4, 44
	orbit.Target = engine.Vector3{Y: 2.45, Z: -1.5}
	cameraTransform := engine.DefaultTransform()
	cameraTransform.Rotation = engine.Vector3{X: -.52, Y: -.31}

	view := world.Add(cameraTransform, projection, orbit)
	if err := world.SetActiveCamera(view); err != nil {
		return nil, err
	}

	sample.FitCamera(app, world, view)

	if err := addFloor(app, world); err != nil {
		return nil, err
	}

	addArchitecture(world)

	ballista, err := addCastle(app, world)
	if err != nil {
		return nil, err
	}

	rig := addLights(world)

	rig.ballista = ballista
	if err := rig.update(); err != nil {
		return nil, err
	}

	rig.bind(app)

	return world, nil
}

//nolint:mnd // Geometry positions and material colors form the authored courtyard.
func addArchitecture(world *engine.World) {
	column := surface(engine.Color{R: .37, G: .41, B: .49, A: 1})
	trim := surface(engine.Color{R: .26, G: .31, B: .38, A: 1})

	for _, block := range courtyard.Pillars() {
		body := mesh.NewBox("pillar", mesh.BoxOptions{Width: block.Width, Height: block.Height, Depth: block.Depth})
		world.Add(body, at(block.X, block.Height/2, block.Z), column)
		crown := mesh.NewBox("pillar cap", mesh.BoxOptions{Width: 1.65, Height: .28, Depth: 1.65})
		world.Add(crown, at(block.X, block.Height, block.Z), trim)
	}

	base := mesh.NewBox("courtyard foundation", mesh.BoxOptions{
		Width: 2 * courtyard.HalfWidth, Height: .62, Depth: 2 * courtyard.HalfDepth,
	})
	world.Add(base, at(0, -.32, 0), surface(engine.Color{R: .27, G: .32, B: .41, A: 1}))
}

func at(x, y, z float32) engine.Transform {
	result := engine.DefaultTransform()
	result.Position = engine.Vector3{X: x, Y: y, Z: z}

	return result
}

//nolint:mnd // A cool ambient fill keeps the unlit castle readable.
func surface(color engine.Color) material.SourceMaterial {
	fill := engine.Color{R: .075, G: .095, B: .145, A: 1}

	result := material.DefaultSource()
	result.Color = color
	result.AmbientColors = [3]engine.Color{fill, fill, fill}
	result.HalfLambert = true
	result.SSBumpStrength = 0

	return result
}
