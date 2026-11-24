package main

import (
	"github.com/karty-game/ashura/pkg/camera"
	"github.com/karty-game/ashura/pkg/engine"
	"github.com/karty-game/ashura/pkg/light"
	"github.com/karty-game/ashura/pkg/material"
	"github.com/karty-game/ashura/pkg/mesh"
	"github.com/karty-game/ashura/pkg/ui"
	"github.com/karty-game/ashura/samples/internal/sample"
	"github.com/karty-game/ashura/samples/internal/sampleui"
)

const (
	panelSize      = 2
	panelElevation = panelSize / 2
	panelSpacing   = 3
	viewDistance   = 11
	skyIntensity   = 0.7
	sunIntensity   = 0.6
	groundSize     = 18
)

func createScene(app *engine.App) (*engine.World, error) {
	world := engine.NewWorld()
	projection, controls := camera.Orbit()
	controls.Distance = viewDistance
	controls.Target = engine.Vector3{Y: 1}

	view := world.Add(engine.DefaultTransform(), projection, controls)
	if err := world.SetActiveCamera(view); err != nil {
		return nil, err
	}

	sky := light.DefaultHemisphere("sky")
	sky.Intensity = skyIntensity
	world.Add(sky)

	sun := light.DefaultDirectional("sun")
	sun.Direction, sun.Intensity = engine.Vector3{X: -1, Y: -1, Z: -1}, sunIntensity
	world.Add(sun)

	ground := material.DefaultStandard()
	ground.Color = engine.ColorDarkGray()
	world.Add(mesh.NewPlane("ground", mesh.PlaneOptions{Width: groundSize, Height: groundSize}),
		engine.DefaultTransform(), ground)

	sample.FitCamera(app, world, view)
	live, _ := engine.Get[camera.OrbitControls](world, view)

	reset := func() {
		live.Distance = viewDistance
		live.Target = engine.Vector3{Y: 1}
		*world.Transform(view) = engine.DefaultTransform()
	}
	if err := sampleui.Controls(
		app,
		"Billboards",
		"Red: spherical · Green: world Y axis · Blue: fixed. Drag to orbit; wheel or pinch to zoom.",
		ui.NewButton("Reset view", reset),
	); err != nil {
		return nil, err
	}

	addBillboards(world)

	return world, nil
}

func addBillboards(world *engine.World) {
	panels := []struct {
		name  string
		mode  mesh.BillboardMode
		color engine.Color
	}{
		{name: "spherical", mode: mesh.BillboardSpherical, color: engine.ColorRed()},
		{name: "world-Y axis", mode: mesh.BillboardAxis, color: engine.ColorGreen()},
		{name: "fixed", mode: mesh.BillboardFixed, color: engine.ColorBlue()},
	}
	for index, panel := range panels {
		surface := material.DefaultStandard()
		surface.Color = panel.color
		world.Add(mesh.NewBillboard(panel.name, mesh.BillboardOptions{
			Width: panelSize, Height: panelSize, Mode: panel.mode,
		}), engine.Transform{
			Position: engine.Vector3{X: float32(index-1) * panelSpacing, Y: panelElevation}, Scale: engine.Vector3One(),
		}, surface)
	}
}
