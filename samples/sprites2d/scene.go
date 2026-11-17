package main

import (
	"errors"
	"fmt"
	"time"

	"github.com/karty-game/karty-webgpu/pkg/engine"
	"github.com/karty-game/karty-webgpu/pkg/texture"
	"github.com/karty-game/karty-webgpu/samples/sprites2d/generated/assets"
	mobile "github.com/karty-game/karty-webgpu/samples/sprites2d/generated/assets/mobile"
)

var errCompression = errors.New("sprites2d requires BC or ETC2 texture compression")

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

		images, err := loadImages(app.CompressedAlbedoFormat())
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

func loadImages(format texture.Format) (assets.Catalog, error) {
	if format == texture.BC1 {
		return assets.Load()
	}

	if format == texture.ETC2RGB {
		images, err := mobile.Load()

		return assets.Catalog(images), err
	}

	return assets.Catalog{}, errCompression
}
