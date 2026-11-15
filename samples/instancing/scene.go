package main

import (
	"math"
	"strconv"
	"time"

	"github.com/karty-game/karty-webgpu/pkg/camera"
	"github.com/karty-game/karty-webgpu/pkg/engine"
	"github.com/karty-game/karty-webgpu/pkg/light"
	"github.com/karty-game/karty-webgpu/pkg/material"
	"github.com/karty-game/karty-webgpu/pkg/mesh"
	"github.com/karty-game/karty-webgpu/pkg/ui"
	"github.com/karty-game/karty-webgpu/samples/internal/sample"
	"github.com/karty-game/karty-webgpu/samples/internal/sampleui"
)

const (
	columns       = 100
	rows          = 50
	spacing       = 1.15
	cameraHeight  = 45
	cameraBack    = 85
	cameraPitch   = -0.49
	skyIntensity  = 0.8
	keyIntensity  = 1.2
	cubeSize      = 0.78
	cubeRoughness = 0.38
	columnPhase   = 0.24
	rowPhase      = 0.27
	waveSpeed     = 1.7
	waveHeight    = 1.8
	spinSpeed     = 0.35
	spinOffset    = 0.025
)

func createScene(app *engine.App) (*engine.World, error) {
	world := engine.NewWorld()
	projection, controls := camera.FirstPerson()

	view := world.Add(
		engine.Transform{
			Position: engine.Vector3{Y: cameraHeight, Z: cameraBack},
			Rotation: engine.Vector3{X: cameraPitch},
			Scale:    engine.Vector3One(),
		},
		projection,
		controls,
	)
	if err := world.SetActiveCamera(view); err != nil {
		return nil, err
	}

	sample.FitCamera(app, world, view)

	sky := light.DefaultHemisphere("sky")
	sky.Intensity = skyIntensity
	world.Add(sky)

	key := light.DefaultDirectional("key")
	key.Direction, key.Intensity = engine.Vector3{X: -0.4, Y: -1, Z: -0.3}, keyIntensity
	world.Add(key)

	transforms := addInstances(world)
	if err := animateInstances(app, transforms); err != nil {
		return nil, err
	}

	return world, nil
}

func addInstances(world *engine.World) []*engine.Transform {
	cube := mesh.NewBox("wave-cube", mesh.BoxOptions{Width: cubeSize, Height: cubeSize, Depth: cubeSize})
	surface := material.DefaultStandard()
	surface.Color, surface.Roughness = engine.ColorOrange(), cubeRoughness
	transforms := make([]*engine.Transform, 0, columns*rows)
	totalWidth := float32(columns-1) * spacing
	totalDepth := float32(rows-1) * spacing

	for row := range rows {
		for column := range columns {
			index := row*columns + column
			instance := cube.CreateInstance("wave-cube" + strconv.Itoa(index))
			baseY := float32(math.Sin(float64(column)*columnPhase) * math.Cos(float64(row)*rowPhase) * waveHeight)
			entity := world.Add(
				instance,
				engine.Transform{
					Position: engine.Vector3{X: float32(column)*spacing - totalWidth/2, Y: baseY, Z: float32(row)*spacing - totalDepth/2},
					Scale:    engine.Vector3One(),
				},
				surface,
			)
			transforms = append(transforms, world.Transform(entity))
		}
	}

	return transforms
}

func animateInstances(app *engine.App, transforms []*engine.Transform) error {
	var elapsed float64

	paused, pauseHeld := false, false

	motion := ui.NewToggle("Animate instances · T", true, func(change ui.Change) { paused = !change.Checked })
	if err := sampleui.Controls(
		app,
		"Instancing",
		"5,000 instances · one shared cube mesh and material. Click to look; WASD moves.",
		motion,
	); err != nil {
		return err
	}

	app.OnUpdate(func(delta time.Duration) {
		down := app.Input().KeyDown(engine.Key('T'))
		if down && !pauseHeld {
			paused = !paused
			motion.SetChecked(!paused)
		}

		pauseHeld = down

		if paused {
			return
		}

		elapsed += delta.Seconds()

		for index, transform := range transforms {
			if transform == nil {
				continue
			}

			column, row := index%columns, index/columns
			phase := elapsed*waveSpeed - float64(column)*columnPhase - float64(row)*rowPhase
			transform.Position.Y = float32(math.Sin(phase) * waveHeight)
			transform.Rotation.Y = float32(elapsed*spinSpeed + float64(column+row)*spinOffset)
		}
	})

	return nil
}
