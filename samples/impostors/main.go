// The impostors sample compares a retained CC0 scan with its baked card.
package main

import (
	"log"

	"github.com/karty-game/karty-webgpu/pkg/debug"
	"github.com/karty-game/karty-webgpu/pkg/engine"
	"github.com/karty-game/karty-webgpu/pkg/engine/features"
	"github.com/karty-game/karty-webgpu/samples/internal/sample"
)

const (
	windowWidth  = 1280
	windowHeight = 720
)

func main() {
	if err := run(); err != nil {
		log.Print(err)
	}
}

func run() (err error) {
	app := engine.NewApp(engine.AppConfig{
		Title:    "Ashura / Impostors",
		CanvasID: "game", Width: windowWidth, Height: windowHeight,
	})
	defer sample.Cleanup(&err, app.Close)

	if err := app.Register(features.Standard(), features.Impostors()); err != nil {
		return err
	}

	world, comparison, err := createScene(app)
	if err != nil {
		return err
	}

	release, err := setupComparison(app, world, comparison)
	if err != nil {
		return err
	}
	defer sample.Cleanup(&err, release)

	panel, err := debug.Show(app)
	if err != nil {
		return err
	}

	panel.Hide()

	return app.Run(world)
}
