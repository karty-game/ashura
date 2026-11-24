// Package sampleui supplies optional retained controls for runnable samples.
package sampleui

import (
	"github.com/karty-game/ashura/pkg/engine"
	"github.com/karty-game/ashura/pkg/ui"
	"github.com/karty-game/ashura/pkg/ui/app"
	"github.com/karty-game/ashura/pkg/ui/fonts/defaultfont"
)

// Controls mounts a compact retained explanation and controls. Scene setup stays
// in each sample; the panel shares its View with the optional diagnostics.
//
//nolint:mnd // Shared sample presentation uses logical pixels and viewport bounds.
func Controls(app *engine.App, title, help string, children ...*ui.Node) error {
	view := uiapp.View(app)
	if view == nil {
		font, err := defaultfont.Load()
		if err != nil {
			return err
		}

		view, err = ui.New(ui.Column(), font)
		if err != nil {
			return err
		}

		if err := uiapp.Attach(app, view); err != nil {
			return err
		}
	}

	content := make([]*ui.Node, 0, 2+len(children))
	content = append(content,
		ui.NewText(title).WithStyle(ui.Style{FontSize: 20}),
		ui.NewText(help).WithStyle(ui.Style{FontSize: 13, Foreground: ui.DarkTheme().Muted}),
	)
	content = append(content, children...)
	panel := ui.Column(content...).
		WithLabel(title, help).
		WithStyle(ui.Style{Padding: 12, Gap: 8, Radius: 12, Fill: ui.DarkTheme().Surface, Scroll: true})
	_, err := view.AddOverlay(
		panel,
		ui.OverlayOptions{Anchor: ui.OverlayBottomLeft, Width: 420, MaxHeightRatio: .35, Margin: 12, Cursor: true},
	)

	return err
}

// Details keeps advanced settings out of the initial scene view.
func Details(label string, children ...*ui.Node) *ui.Node {
	content := ui.Column(children...)
	content.SetHidden(true)

	toggle := ui.NewSwitch(label, false, func(change ui.Change) { content.SetHidden(!change.Checked) })

	return ui.Column(toggle, content)
}
