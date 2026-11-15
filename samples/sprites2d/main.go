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
	windowHeight = 800
)

func main() {
	if err := run(); err != nil {
		log.Print(err)
	}
}

func run() (err error) {
	app := engine.NewApp(engine.AppConfig{
		Title: "Ashura / Sprites2D — Abyssal", CanvasID: "game", Width: windowWidth, Height: windowHeight,
	})
	defer sample.Cleanup(&err, app.Close)

	if err := app.Register(features.Sprites()); err != nil {
		return err
	}

	world, err := createScene(app)
	if err != nil {
		return err
	}

	panel, err := debug.Show(app)
	if err != nil {
		return err
	}

	panel.Hide()

	return app.Run(world)
}
