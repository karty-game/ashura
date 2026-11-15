package main

import (
	"time"

	"github.com/karty-game/karty-webgpu/pkg/engine"
	"github.com/karty-game/karty-webgpu/pkg/ui"
	"github.com/karty-game/karty-webgpu/samples/internal/sampleui"
)

type patternAnimation struct {
	elapsed              float64
	paused, spaceWasDown bool
}

func (animation *patternAnimation) attach(app *engine.App) error {
	motion := ui.NewToggle("Animate texture · Space", true, func(change ui.Change) { animation.paused = !change.Checked })
	if err := sampleui.Controls(
		app,
		"Compute texture",
		"WGSL writes the shared texture on the GPU. Drag to orbit; wheel or pinch to zoom.",
		motion,
	); err != nil {
		return err
	}

	app.OnUpdate(func(delta time.Duration) {
		spaceDown := app.Input().KeyDown(engine.KeySpace)
		if spaceDown && !animation.spaceWasDown {
			animation.paused = !animation.paused
			motion.SetChecked(!animation.paused)
		}

		animation.spaceWasDown = spaceDown

		if !animation.paused {
			animation.elapsed += delta.Seconds()
		}
	})

	return nil
}
