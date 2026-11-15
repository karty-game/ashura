//nolint:mnd // The sample declares its initial window dimensions explicitly.
package main

import (
	"log"

	"github.com/karty-game/karty-webgpu/pkg/debug"
	"github.com/karty-game/karty-webgpu/pkg/engine"
	"github.com/karty-game/karty-webgpu/samples/internal/sample"
)

func main() {
	if err := run(); err != nil {
		log.Print(err)
	}
}

func run() (err error) {
	app := engine.NewApp(engine.AppConfig{Title: "Ashura / UI Components", CanvasID: "game", Width: 1120, Height: 850})
	defer sample.Cleanup(&err, app.Close)

	if err := setupUI(app); err != nil {
		return err
	}

	panel, err := debug.Show(app)
	if err != nil {
		return err
	}

	panel.Hide()

	return app.Run(engine.NewWorld())
}
