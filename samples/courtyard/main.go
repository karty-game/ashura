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
	canvasID     = "game"
)

func main() {
	if err := run(); err != nil {
		log.Print(err)
		showCourtyardError(err)
	}
}

func run() (err error) {
	app := engine.NewApp(engine.AppConfig{
		Title: "Ashura / Collision courtyard", CanvasID: canvasID, Width: windowWidth, Height: windowHeight,
	})
	defer sample.Cleanup(&err, app.Close)

	if err := app.Register(features.Standard()); err != nil {
		return err
	}

	art, err := loadCourtyardArt(app)
	if err != nil {
		return err
	}

	scene, err := buildCourtyard(art)
	if err != nil {
		return err
	}

	panel, err := debug.Show(app)
	if err != nil {
		return err
	}

	panel.Hide()

	cleanup := scene.bindControls(app, canvasID, panel)
	defer cleanup()

	return app.Run(scene.world)
}
