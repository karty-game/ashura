//nolint:testableexamples // The interactive example is compiled; execution needs a real platform backend.
package engine_test

import (
	"errors"
	"log"
	"time"

	"github.com/karty-game/karty-webgpu/pkg/camera"
	"github.com/karty-game/karty-webgpu/pkg/engine"
	"github.com/karty-game/karty-webgpu/pkg/engine/features"
	"github.com/karty-game/karty-webgpu/pkg/light"
	"github.com/karty-game/karty-webgpu/pkg/material"
	"github.com/karty-game/karty-webgpu/pkg/mesh"
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
