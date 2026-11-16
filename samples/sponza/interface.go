//nolint:mnd // The navigation panel uses authored logical-pixel dimensions.
package main

import (
	"github.com/karty-game/karty-webgpu/pkg/engine"
	"github.com/karty-game/karty-webgpu/pkg/ui"
	"github.com/karty-game/karty-webgpu/pkg/ui/app"
	"github.com/karty-game/karty-webgpu/pkg/ui/fonts/defaultfont"
	"github.com/karty-game/karty-webgpu/samples/internal/sampleui"
)

type navigationUI struct {
	status *ui.Node
	remove func()
}

func showNavigation(app *engine.App, selectView func(int)) (*navigationUI, error) {
	view := uiapp.View(app)
	if view == nil {
		font, err := defaultfont.Load()
		if err != nil {
			return nil, err
		}

		view, err = ui.New(ui.Column(), font)
		if err != nil {
			return nil, err
		}

		if err := uiapp.Attach(app, view); err != nil {
			return nil, err
		}
	}

	status := ui.NewText("")
	settings := sampleui.Details("Views",
		ui.Row(
			ui.NewButton("Courtyard · 1", func() { selectView(courtyardView) }),
			ui.NewButton("Gallery · 2", func() { selectView(galleryView) }),
			ui.NewButton("Free flight · 3", func() { selectView(flightView) }),
			ui.NewButton("Ceiling · B", func() { selectView(ceilingView) }),
		).WithStyle(ui.Style{Wrap: true, Gap: 6}),
		ui.NewText("Drag to look · wheel/pinch to zoom · R resets · Flight: WASD, Space/Shift"),
	)
	controls := ui.Column(ui.NewText("Sponza").WithStyle(ui.Style{FontSize: 20}), status, settings, ui.NewText("© Frank Meinl, Crytek · Sponza").WithStyle(ui.Style{FontSize: 12})).
		WithLabel("Sponza views and baked lighting", "Camera controls and prepared lighting status").
		WithStyle(ui.Style{Padding: 12, Gap: 8, Radius: 12, Fill: ui.DarkTheme().Surface, Scroll: true})

	remove, err := view.AddOverlay(controls, ui.OverlayOptions{
		Anchor: ui.OverlayBottomLeft, Width: 760, MaxHeightRatio: .4, Margin: 12, Cursor: true,
	})
	if err != nil {
		return nil, err
	}

	return &navigationUI{status: status, remove: remove}, nil
}
