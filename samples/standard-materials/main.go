package main

import (
	"log"

	"github.com/karty-game/ashura/pkg/debug"
	"github.com/karty-game/ashura/pkg/engine"
	"github.com/karty-game/ashura/pkg/engine/features"
	"github.com/karty-game/ashura/samples/internal/sample"
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
	app := engine.NewApp(
		engine.AppConfig{
			Title:    "Ashura / Materials",
			CanvasID: "game",
			Width:    windowWidth,
			Height:   windowHeight,
		},
	)
	defer sample.Cleanup(&err, app.Close)

	if err := app.Register(features.Standard(), features.Source()); err != nil {
		return err
	}

	scene, err := createScene(app)
	if err != nil {
		return err
	}

	panel, err := debug.Show(app)
	if err != nil {
		return err
	}

	panel.Hide()

	return app.Run(scene)
}
