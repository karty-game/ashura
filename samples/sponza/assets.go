package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/karty-game/karty-webgpu/pkg/engine"
	"github.com/karty-game/karty-webgpu/pkg/level"
	"github.com/karty-game/karty-webgpu/pkg/scene/resource"
	"github.com/karty-game/karty-webgpu/samples/internal/sample"
	"github.com/karty-game/karty-webgpu/samples/sponza/generated/levels"
)

const rgbRepresentation = "RGB"

type bakedPresentation struct {
	app            *engine.App
	world          *engine.World
	baked          *level.Presentation
	rgb, activeRGB bool
	changed        func(string)
	failures       sample.Errors
}

func createScene(app *engine.App) (*engine.World, func() error, error) {
	world, selectView, err := createWorld(app)
	if err != nil {
		return nil, nil, err
	}

	presentation := &bakedPresentation{app: app, world: world}
	if err = presentation.applyLighting(); err != nil {
		return nil, nil, err
	}

	navigation, err := showNavigation(app, selectView, func(rgb bool) { presentation.rgb = rgb })
	if err != nil {
		return nil, nil, errors.Join(err, presentation.baked.Close())
	}

	presentation.changed = func(text string) { presentation.failures.Check(navigation.status.SetText(text)) }
	presentation.notify(presentation.baked.Status())
	stopUpdate := app.OnUpdateError(presentation.update)

	return world, func() (err error) {
		stopUpdate()
		navigation.remove()

		presentation.changed = nil
		sample.Cleanup(&err, presentation.baked.Close)

		return err
	}, nil
}

func (presentation *bakedPresentation) update(delta time.Duration) error {
	if err := presentation.failures.Err(); err != nil {
		return err
	}

	if err := presentation.applyLighting(); err != nil {
		return err
	}

	return presentation.baked.Update(delta)
}

// Lighting selection changes only the prepared profile. The standard level API
// owns loading, cancellation, verified resource sharing and presentation lifetime.
func (presentation *bakedPresentation) applyLighting() error {
	if presentation.baked != nil {
		if presentation.activeRGB == presentation.rgb {
			return nil
		}

		if err := presentation.baked.Close(); err != nil {
			return err
		}
	}

	profiles := level.TextureVariants{BC: levels.SponzaRNMBC(), Mobile: levels.SponzaRNMMobile()}
	if presentation.rgb {
		profiles = level.TextureVariants{BC: levels.SponzaRGBBC(), Mobile: levels.SponzaRGBMobile()}
	}

	opened, err := level.Open(context.Background(), presentation.app, presentation.world, level.OpenOptions{
		Variants: profiles, Base: resource.ProjectBase("samples/sponza"), Select: level.WholeScene, StatusChanged: presentation.notify,
	})
	if err != nil {
		return err
	}

	presentation.baked, presentation.activeRGB = opened, presentation.rgb
	presentation.notify(opened.Status())

	return nil
}

func (presentation *bakedPresentation) notify(status level.PresentationStatus) {
	if presentation.changed == nil {
		return
	}

	representation := "RNM"
	if presentation.activeRGB {
		representation = rgbRepresentation
	}

	presentation.changed(
		fmt.Sprintf(
			"%s lighting · %d/%d chunks ready",
			representation,
			status.LoadedChunks,
			status.TotalChunks,
		),
	)
}
