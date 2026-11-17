//nolint:mnd // The editorial layout uses authored logical-pixel dimensions and colors.
package main

import (
	"fmt"

	"github.com/karty-game/karty-webgpu/pkg/ui"
	"github.com/karty-game/karty-webgpu/pkg/ui/app"
	"github.com/karty-game/karty-webgpu/pkg/ui/fonts/defaultfont"
)

const amberPalette = "amber"

func (study *expedition) createInterface() error {
	font, err := defaultfont.Load()
	if err != nil {
		return fmt.Errorf("load expedition font: %w", err)
	}

	view, err := ui.New(ui.Column(), font)
	if err != nil {
		return err
	}

	study.view = view
	view.OnPreview(study.pointSonar)

	if err := view.SetTheme(oceanTheme()); err != nil {
		return err
	}

	study.title = ui.NewText("Abyssal").WithStyle(ui.Style{FontSize: 48, Foreground: ui.Hex(0xe4f4ed)})

	heading := ui.Column(study.title)
	if _, err := view.AddOverlay(
		heading,
		ui.OverlayOptions{Anchor: ui.OverlayTopLeft, Width: 250, Margin: 28, MaxHeightRatio: .2},
	); err != nil {
		return err
	}

	if _, err := view.AddOverlay(
		study.controls(),
		ui.OverlayOptions{Anchor: ui.OverlayBottomLeft, Width: 410, Margin: 28, MaxHeightRatio: .4, Cursor: true},
	); err != nil {
		return err
	}

	study.readout = ui.NewText("").WithStyle(ui.Style{FontSize: 13, Foreground: ui.Hex(0x82b4b7)})

	study.footer = ui.Column(study.readout).
		WithStyle(ui.Style{Padding: 12, Radius: 10, Fill: ui.Color{.025, .09, .12, .92}})
	if _, err := view.AddOverlay(
		study.footer,
		ui.OverlayOptions{Anchor: ui.OverlayBottomRight, Width: 225, Margin: 28, MaxHeightRatio: .2},
	); err != nil {
		return err
	}

	return uiapp.Attach(study.app, view)
}

func oceanTheme() ui.Theme {
	theme := ui.DarkTheme()
	theme.Surface = ui.Hex(0x09232d)
	theme.Control = ui.Hex(0x14343d)
	theme.Hover = ui.Hex(0x1c4750)
	theme.Pressed = ui.Hex(0x24515a)
	theme.Border = ui.Hex(0x25505a)
	theme.Accent = ui.Hex(0x8bdec5)
	theme.Foreground = ui.Hex(0xd8eae5)
	theme.Muted = ui.Hex(0x83acb1)
	theme.FontSize = 13
	theme.ControlHeight = 36
	theme.Radius = 8
	theme.Gap = 8

	return theme
}

func (study *expedition) controls() *ui.Node {
	study.motion = ui.NewToggle("Motion", true, func(change ui.Change) { study.setPaused(!change.Checked) })
	depth := ui.NewToggle("Depth", true, func(change ui.Change) { study.flat = !change.Checked; study.applyLayers(); study.dirty = true })

	scan := ui.NewToggle("Scan", false, func(change ui.Change) { study.scanned = change.Checked; study.layoutScan(); study.dirty = true })
	for _, node := range []*ui.Node{study.motion, depth, scan} {
		node.WithStyle(ui.Style{Width: 112, Height: 36})
	}

	palettes := ui.NewSelect("Bioluminescence", []ui.Option{
		{Key: "blue", Label: "Blue hour"}, {Key: "green", Label: "Sea glass"}, {Key: amberPalette, Label: "Amber"},
	}, "blue", func(change ui.Change) {
		index := 0

		switch change.Text {
		case "green":
			index = 1
		case amberPalette:
			index = 2
		}

		study.setPalette(index)
	})

	return ui.Column(
		ui.Row(ui.NewText("Controls").WithStyle(ui.Style{Grow: 1, FontSize: 11}),
			ui.NewText("Space: pause").WithStyle(ui.Style{FontSize: 10, Foreground: ui.Hex(0x6f989f)})),
		ui.Row(study.motion, depth, scan).WithStyle(ui.Style{Gap: 8, Wrap: true}),
		palettes,
	).WithStyle(ui.Style{Padding: 18, Gap: 12, Radius: 14, Fill: ui.Color{.025, .09, .12, .92}, Border: ui.Hex(0x23454e), BorderWidth: 1, Scroll: true})
}

func (study *expedition) setPaused(paused bool) {
	study.paused = paused
	study.motion.SetChecked(!paused)
	study.dirty = true
}

func (study *expedition) updateReadout() error {
	stats := study.screen.Stats()

	return study.readout.SetText(
		fmt.Sprintf("%d sprites · %d pages", stats.Sprites, stats.Pages),
	)
}

func (study *expedition) responsive() {
	bounds := study.view.Root().Bounds()
	compact := bounds.W > 0 && bounds.W < 760

	fontSize := float32(48)
	if compact {
		fontSize = 34
	}

	study.title.WithStyle(ui.Style{FontSize: fontSize, Foreground: ui.Hex(0xe4f4ed)})
	study.footer.SetHidden(compact)
}
