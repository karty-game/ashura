package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/karty-game/ashura/pkg/engine"
	"github.com/karty-game/ashura/pkg/level"
	"github.com/karty-game/ashura/pkg/scene/resource"
	"github.com/karty-game/ashura/samples/internal/sample"
	"github.com/karty-game/ashura/samples/sponza/generated/levels"
)

type bakedPresentation struct {
	baked    *level.Presentation
	changed  func(string)
	failures sample.Errors
}

func createScene(app *engine.App) (*engine.World, func() error, error) {
	world, selectView, err := createWorld(app)
	if err != nil {
		return nil, nil, err
	}

	presentation, err := openBakedScene(app, world)
	if err != nil {
		return nil, nil, err
	}

	navigation, err := showNavigation(app, selectView)
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

	return presentation.baked.Update(delta)
}

// The level API selects the supported GPU texture family for the fixed RNM scene
// and owns prepared resource loading, sharing and presentation lifetime.
func openBakedScene(app *engine.App, world *engine.World) (*bakedPresentation, error) {
	presentation := &bakedPresentation{}

	opened, err := level.Open(context.Background(), app, world, level.OpenOptions{
		Variants: level.TextureVariants{BC: levels.SponzaRNMBC(), Mobile: levels.SponzaRNMMobile()},
		Base:     resource.ProjectBase("samples/sponza"), Select: level.WholeScene, StatusChanged: presentation.notify,
	})
	if err != nil {
		return nil, err
	}

	presentation.baked = opened

	return presentation, nil
}

func (presentation *bakedPresentation) notify(status level.PresentationStatus) {
	if presentation.changed == nil {
		return
	}

	presentation.changed(
		fmt.Sprintf(
			"RNM lighting · %d/%d chunks ready",
			status.LoadedChunks,
			status.TotalChunks,
		),
	)
}
