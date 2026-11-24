//nolint:testableexamples // The interactive example is compiled; execution needs a real platform backend.
package engine_test

import (
	"errors"
	"log"
	"math"
	"time"

	"github.com/karty-game/ashura/pkg/camera"
	"github.com/karty-game/ashura/pkg/engine"
	"github.com/karty-game/ashura/pkg/engine/features"
	"github.com/karty-game/ashura/pkg/light"
	"github.com/karty-game/ashura/pkg/material"
	"github.com/karty-game/ashura/pkg/mesh"
)

// ExampleApp is compiled by go test. It has no Output directive because running
// it opens the real platform backend and enters the interactive frame loop.
func ExampleApp() {
	if err := firstScene(); err != nil {
		log.Print(err)
	}
}

func firstScene() (err error) {
	app := engine.NewApp(engine.AppConfig{
		Title: "First scene", Width: 1280, Height: 720, CanvasID: "game",
	})
	defer func() { err = errors.Join(err, app.Close()) }()

	if err := app.Register(features.Standard()); err != nil {
		return err
	}

	world := engine.NewWorld()
	projection, controls := camera.Orbit()

	view := world.Add(engine.DefaultTransform(), projection, controls)
	if err := world.SetActiveCamera(view); err != nil {
		return err
	}

	world.Add(light.DefaultHemisphere("sky"))

	surface := material.DefaultStandard()
	surface.Color = engine.ColorOrange()
	surface.Roughness = 0.6
	box := world.Add(mesh.NewBox("box", mesh.BoxOptions{}), engine.DefaultTransform(), surface)

	stop := app.OnUpdate(func(dt time.Duration) {
		world.Transform(box).Rotation.Y += float32(dt.Seconds())
	})
	defer stop()

	return app.Run(world)
}

// ExampleApp_dynamicLights animates a point light in a scene with a spotlight
// and a floor that receives both lights' shadows.
func ExampleApp_dynamicLights() {
	app := engine.NewApp(engine.AppConfig{Title: "Dynamic lights", Width: 1280, Height: 720, CanvasID: "game"})
	defer func() { _ = app.Close() }()

	if err := app.Register(features.Standard()); err != nil {
		log.Print(err)

		return
	}

	world := engine.NewWorld()
	projection, orbit := camera.Orbit()
	orbit.Distance = 8
	view := engine.DefaultTransform()
	view.Rotation.X = -.45
	world.Add(view, projection, orbit)
	world.Add(mesh.NewBox("box", mesh.BoxOptions{}), engine.DefaultTransform(), material.DefaultStandard())

	if err := addMovingLights(app, world); err != nil {
		log.Print(err)

		return
	}

	if err := app.Run(world); err != nil {
		log.Print(err)
	}
}

func addMovingLights(app *engine.App, world *engine.World) error {
	if err := app.Register(features.ClusteredLights(), features.PointShadows()); err != nil {
		return err
	}

	floor := engine.DefaultTransform()
	floor.Position.Y = -.6
	world.Add(mesh.NewBox("floor", mesh.BoxOptions{Width: 10, Height: .2, Depth: 10}), floor, material.DefaultStandard())

	point := light.DefaultPoint("amber")
	point.Position = engine.Vector3{X: 2, Y: 2, Z: 1}
	point.Color = engine.ColorOrange()
	point.Range = 6
	point.CastShadows = true
	mover, _ := engine.Get[light.Point](world, world.Add(point))

	spot := light.DefaultSpot("overhead")
	spot.Position = engine.Vector3{Y: 4}
	spot.CastShadows = true
	world.Add(spot)

	var elapsed float64

	app.OnUpdate(func(dt time.Duration) {
		elapsed += dt.Seconds()
		mover.Position.X = 2 * float32(math.Cos(elapsed))
		mover.Position.Z = 2 * float32(math.Sin(elapsed))
	})

	return nil
}
