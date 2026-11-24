package main

import (
	"fmt"
	"time"

	"github.com/karty-game/ashura/pkg/engine"
	"github.com/karty-game/ashura/samples/sprites2d/generated/assets"
)

func createScene(app *engine.App) (*engine.World, error) {
	study := newExpedition(app)
	if err := study.createInterface(); err != nil {
		return nil, err
	}

	// The device is available only after Run opens it. Select and admit exactly
	// one prepared catalog on the first update, then remove this callback.
	var stop func()

	stop = app.OnUpdateError(func(delta time.Duration) error {
		stop()

		images, err := assets.LoadForDevice(app.SupportsTextureFormat)
		if err != nil {
			return fmt.Errorf("load expedition artwork: %w", err)
		}

		if err := study.createSprites(images); err != nil {
			return err
		}

		app.OnUpdateError(study.update)

		return study.update(delta)
	})

	return engine.NewWorld(), nil
}
