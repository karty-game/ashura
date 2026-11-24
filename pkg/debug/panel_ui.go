//nolint:mnd // The debug overlay uses explicit bounded logical-pixel dimensions.
package debug

import (
	"fmt"

	"github.com/karty-game/ashura/pkg/engine"
	"github.com/karty-game/ashura/pkg/ui"
	"github.com/karty-game/ashura/pkg/ui/app"
	"github.com/karty-game/ashura/pkg/ui/fonts/defaultfont"
)

type panelUI struct {
	view                                  *ui.View
	section, toggle                       *ui.Node
	rows                                  [rowCount]*ui.Node
	text                                  [rowCount]string
	scratch                               [64]byte
	removeSection, removeToggle, stopKeys func()
	failure                               error
}

func (widgets *panelUI) open(app *engine.App, panel *Panel) error {
	view := uiapp.View(app)

	created := view == nil
	if created {
		font, err := defaultfont.Load()
		if err != nil {
			return fmt.Errorf("debug font: %w", err)
		}

		view, err = ui.New(ui.Column(), font)
		if err != nil {
			return fmt.Errorf("debug surface: %w", err)
		}
	}

	widgets.view = view
	if err := widgets.build(panel); err != nil {
		return err
	}

	var err error

	widgets.removeSection, err = view.AddOverlay(widgets.section, ui.OverlayOptions{
		Anchor: ui.OverlayTopRight, Width: 360, MaxHeightRatio: .7, Margin: 12, Cursor: true,
	})
	if err != nil {
		return fmt.Errorf("debug panel: %w", err)
	}

	widgets.removeToggle, err = view.AddOverlay(widgets.toggle, ui.OverlayOptions{
		Anchor: ui.OverlayTopRight, Width: 118, Margin: 12,
	})
	if err != nil {
		widgets.close()

		return fmt.Errorf("debug toggle: %w", err)
	}

	widgets.stopKeys = view.OnPreview(panel.shortcut)
	if created {
		if err := uiapp.Attach(app, view); err != nil {
			widgets.close()

			return err
		}
	}

	return nil
}

func (widgets *panelUI) build(panel *Panel) error {
	theme := widgets.view.Theme()
	header := ui.Row(
		ui.NewText("Engine debug").WithStyle(ui.Style{Grow: 1, FontSize: 16}),
		ui.NewButton("Hide · F3", panel.Hide).
			WithLabel("Hide debug panel", "F3 toggles the panel; Escape hides it").
			WithStyle(ui.Style{Width: 96, Height: 44, FontSize: 13}),
	).WithStyle(ui.Style{Height: 44, Gap: 8})

	body := widgets.buildRows(theme)
	if err := body.SetFocusable(true); err != nil {
		return err
	}

	widgets.section = ui.Column(header, body).WithStyle(ui.Style{
		Padding: 12, Gap: 8, Fill: theme.Surface, Border: theme.Border, BorderWidth: 1, Radius: theme.Radius,
	})
	widgets.section.SetHidden(true)
	widgets.toggle = ui.NewButton("Debug · F3", panel.Show).
		WithLabel("Show debug panel", "F3 toggles the panel").
		WithStyle(ui.Style{Height: 44, FontSize: 13})
	widgets.toggle.SetHidden(true)

	return nil
}

func (widgets *panelUI) show(visible bool) {
	widgets.section.SetHidden(!visible)
	widgets.toggle.SetHidden(visible)
}

func (widgets *panelUI) close() {
	for _, stop := range []func(){widgets.stopKeys, widgets.removeToggle, widgets.removeSection} {
		if stop != nil {
			stop()
		}
	}

	widgets.stopKeys, widgets.removeToggle, widgets.removeSection = nil, nil, nil
	widgets.section, widgets.toggle, widgets.view = nil, nil, nil
	widgets.rows = [rowCount]*ui.Node{}
	widgets.text = [rowCount]string{}
}

func (panel *Panel) shortcut(event *ui.RoutedEvent) {
	if event.Event.Kind != ui.KeyDown {
		return
	}

	switch event.Event.Key {
	case "F3":
		event.PreventDefault()
		event.StopPropagation()

		if event.Event.Repeat {
			return
		}

		if panel.Active() {
			panel.Hide()
		} else {
			panel.Show()
		}
	case "Escape":
		if panel.Active() && !panel.ui.view.CapturesKeyboard() {
			panel.Hide()
			event.PreventDefault()
			event.StopPropagation()
		}
	}
}
