package main

import (
	"github.com/karty-game/ashura/pkg/engine"
	"github.com/karty-game/ashura/pkg/ui"
	"github.com/karty-game/ashura/samples/internal/sampleui"
)

func (demo *materialDemo) setupUI(app *engine.App) error {
	demo.mode = ui.NewSwitch("Valve / Source material · T", false, func(change ui.Change) {
		demo.failures.Check(demo.setMode(change.Checked))
	})
	demo.status = ui.NewText("")
	demo.normal = ui.NewCheckbox("Normal mapping · N", true, func(change ui.Change) {
		demo.failures.Check(demo.setNormal(change.Checked))
	})
	demo.occlusion = ui.NewCheckbox("Ambient occlusion · O", true, func(change ui.Change) {
		demo.failures.Check(demo.setOcclusion(change.Checked))
	})
	demo.ssbump = ui.NewCheckbox("SSBump relief · B", true, func(change ui.Change) {
		demo.failures.Check(demo.setSSBump(change.Checked))
	})
	demo.halfLambert = ui.NewCheckbox("Half-Lambert · H", false, func(change ui.Change) {
		demo.failures.Check(demo.setHalfLambert(change.Checked))
	})

	demo.lighting = ui.NewButton("Cycle RNM ambient colors · R", func() {
		demo.failures.Check(demo.cycleLighting())
	})
	if err := demo.syncUI(); err != nil {
		return err
	}

	return sampleui.Controls(
		app,
		"Materials",
		"Standard uses normal maps and AO. Source uses baked directional relief. Click to look; WASD moves.",
		demo.mode,
		demo.status,
		sampleui.Details("Comparison settings",
			demo.normal, demo.occlusion, demo.ssbump, demo.halfLambert, demo.lighting),
	)
}

func (demo *materialDemo) syncUI() error {
	if demo.mode == nil {
		return nil
	}

	pair := demo.surfaces[0].pair
	demo.mode.SetChecked(demo.useSource)
	demo.normal.SetChecked(pair.standard.NormalStrength != 0)
	demo.occlusion.SetChecked(pair.standard.AOStrength != 0)
	demo.ssbump.SetChecked(pair.source.SSBumpStrength != 0)
	demo.halfLambert.SetChecked(pair.source.HalfLambert)
	demo.normal.SetDisabled(demo.useSource)
	demo.occlusion.SetDisabled(demo.useSource)
	demo.ssbump.SetDisabled(!demo.useSource)
	demo.halfLambert.SetDisabled(!demo.useSource)
	demo.lighting.SetDisabled(!demo.useSource)

	label := "Standard: tangent normals, AO and specular"
	if demo.useSource {
		label = "Valve / Source: SSBump and RNM ambient"
	}

	return demo.status.SetText(label)
}
