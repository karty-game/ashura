package main

import (
	"fmt"

	"github.com/karty-game/ashura/pkg/engine"
	"github.com/karty-game/ashura/pkg/ui"
	"github.com/karty-game/ashura/samples/decals/internal/room"
	"github.com/karty-game/ashura/samples/internal/sampleui"
)

type paintStatus struct {
	color            uint8
	painted, details int
	clearing         bool
}

//nolint:mnd // Control names, sizes and swatches are authored presentation values.
func (tool *sprayer) controls(app *engine.App) error {
	status := ui.NewText("").WithStyle(ui.Style{Basis: 120, Grow: 1, FontSize: 13, Foreground: ui.Hex(0xd7ebeb)})
	tool.status = status

	tints := [sprayColors]uint32{0x86fff0, 0xff8cb2, 0xffd382}

	const darkInk = 0x111521

	palette := make([]*ui.Node, 0, sprayColors+1)

	for index, option := range room.Paints() {
		pigment := uint8(index + 1) // The palette has three entries.
		button := ui.NewButton(fmt.Sprintf("%d %s", index+1, option.Name), func() { tool.color = pigment })
		button.WithStyle(
			ui.Style{
				Basis:      62,
				Grow:       1,
				Height:     44,
				Padding:    8,
				FontSize:   14,
				Radius:     5,
				Foreground: ui.Hex(darkInk),
				Fill:       ui.Hex(tints[index]),
			},
		)
		palette = append(palette, button)
	}

	clearPaint := ui.NewButton("Clear", func() { tool.clearPending = true }).
		WithStyle(ui.Style{Basis: 62, Grow: 1, Height: 44, FontSize: 14, Radius: 5})
	palette = append(palette, clearPaint)

	toggle := ui.NewSwitch("Yard decals", true, func(change ui.Change) { tool.wantDetails = change.Checked }).
		WithStyle(ui.Style{Width: 144, Height: 44, FontSize: 14})

	return sampleui.Controls(
		app,
		"Switchyard",
		"Drag to spray. Right-drag to orbit or pan with two fingers. Scroll or pinch to zoom.",
		ui.Row(palette...).WithStyle(ui.Style{Gap: 6, Wrap: true}),
		ui.Row(toggle, status).WithStyle(ui.Style{Gap: 8, Wrap: true}),
	)
}

func (tool *sprayer) updateStatus() error {
	details := 0
	if tool.details != nil {
		details = tool.detailBatch.Count()
	}

	current := paintStatus{color: tool.color, painted: tool.painted, details: details, clearing: tool.clearing}
	if current == tool.shown {
		return nil
	}

	tool.shown = current
	if current.clearing {
		return tool.status.SetText("Clearing paint…")
	}

	paint := room.Paints()[current.color-1].Name

	return tool.status.SetText(fmt.Sprintf("%s · %d painted · %d authored", paint, current.painted, current.details))
}
