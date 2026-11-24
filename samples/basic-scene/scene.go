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
	cameraHeight   = 2.8
	cameraDistance = 8
	cameraPitch    = -0.2

	lightIntensity  = 0.8
	groundSize      = 12
	groundRoughness = 0.9

	sphereSegments = 32
	sphereOffset   = 2
	cubeHalfHeight = 0.5
	cubeRoughness  = 0.45
)

//nolint:mnd // The shared sample palette uses authored linear colors.
func createScene(app *engine.App) (*engine.World, error) {
	scene := engine.NewWorld()
	projection, controls := camera.FirstPerson()

	view := scene.Add(
		engine.Transform{
			Position: engine.Vector3{Y: cameraHeight, Z: cameraDistance},
			Rotation: engine.Vector3{X: cameraPitch},
			Scale:    engine.Vector3One(),
		},
		projection,
		controls,
	)
	if err := scene.SetActiveCamera(view); err != nil {
		return nil, err
	}

	sample.FitCamera(app, scene, view)

	sky := light.DefaultHemisphere("sky")
	sky.Intensity = lightIntensity
	scene.Add(sky)

	groundSurface := material.DefaultStandard()
	groundSurface.Color, groundSurface.Roughness = engine.Color{R: .09, G: .15, B: .19, A: 1}, groundRoughness
	orange := material.DefaultStandard()
	orange.Color, orange.Roughness = engine.Color{R: .75, G: .3, B: .1, A: 1}, cubeRoughness

	ground := mesh.NewPlane("ground", mesh.PlaneOptions{Width: groundSize, Height: groundSize})
	scene.Add(
		ground,
		engine.DefaultTransform(),
		groundSurface,
	)

	cube := mesh.NewBox("cube", mesh.BoxOptions{Width: 1, Height: 1, Depth: 1})
	scene.Add(
		cube,
		engine.Transform{
			Position: engine.Vector3{Y: cubeHalfHeight},
			Scale:    engine.Vector3One(),
		},
		orange,
	)

	sphere := mesh.NewSphere("sphere", mesh.SphereOptions{Diameter: 1, Segments: sphereSegments})
	scene.Add(
		sphere,
		engine.Transform{
			Position: engine.Vector3{X: sphereOffset, Y: cubeHalfHeight},
			Scale:    engine.Vector3One(),
		},
		orange,
	)

	return scene, nil
}
