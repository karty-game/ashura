package main

import (
	"fmt"

	"github.com/karty-game/karty-webgpu/pkg/engine"
	"github.com/karty-game/karty-webgpu/samples/sprites2d/generated/assets"
)

func createScene(app *engine.App) (*engine.World, error) {
	images, err := assets.Load()
	if err != nil {
		return nil, fmt.Errorf("load expedition artwork: %w", err)
	}

	study := newExpedition(app, images)
	if err := study.createInterface(); err != nil {
		return nil, err
	}

	app.OnUpdateError(study.update)

	return engine.NewWorld(), nil
}
