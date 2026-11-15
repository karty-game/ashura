package main

import (
	"github.com/karty-game/karty-webgpu/pkg/engine"
	"github.com/karty-game/karty-webgpu/pkg/material"
	"github.com/karty-game/karty-webgpu/pkg/reflection"
	"github.com/karty-game/karty-webgpu/pkg/ui"
	"github.com/karty-game/karty-webgpu/samples/internal/sampleui"
)

func showReflectionControls(app *engine.App, liveProbe *reflection.Probe, reflective []*material.Material) error {
	toggle := ui.NewSwitch("Reflections", true, func(change ui.Change) {
		liveProbe.Strength = 0
		if change.Checked {
			liveProbe.Strength = 1
		}
	})

	roughness := ui.NewSlider("Surface roughness", .02, .8, .02, .12, func(change ui.Change) {
		for _, surface := range reflective {
			surface.Roughness = float32(change.Value)
		}
	})
	if err := sampleui.Controls(
		app,
		"Reflections",
		"Move to see box-projected reflections. Click to look; WASD moves.",
		toggle,
		sampleui.Details("Surface settings", ui.NewText("Roughness"), roughness),
	); err != nil {
		return err
	}

	return nil
}
