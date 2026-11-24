// The sponza sample explores an offline-baked Sponza with independent runtime cutouts.
package main

import (
	"log"

	"github.com/karty-game/ashura/pkg/debug"
	"github.com/karty-game/ashura/pkg/engine"
	"github.com/karty-game/ashura/pkg/engine/features"
	"github.com/karty-game/ashura/samples/internal/sample"
)

const width, height = 1280, 720

func main() {
	if err := run(); err != nil {
		log.Print(err)
	}
}

func run() (err error) {
	app := engine.NewApp(engine.AppConfig{
		Title:    "Ashura / Sponza",
		CanvasID: "game",
		Width:    width,
		Height:   height,
	})
	defer sample.Cleanup(&err, app.Close)

	if err := app.Register(features.Standard(), features.Source()); err != nil {
		return err
	}

	world, cleanup, err := createScene(app)
	if err != nil {
		return err
	}
	defer sample.Cleanup(&err, cleanup)

	panel, err := debug.Show(app)
	if err != nil {
		return err
	}

	panel.Hide()

	return app.Run(world)
}
