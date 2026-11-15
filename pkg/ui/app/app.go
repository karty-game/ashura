// Package uiapp explicitly connects retained UI to an engine App.
// Programs that do not attach UI need not import it or the text-shaping stack.
package uiapp

import (
	"github.com/karty-game/karty-webgpu/pkg/engine"
	"github.com/karty-game/karty-webgpu/pkg/ui"
)

type attachment struct{ *ui.View }

// Attach installs view before the App starts. A nil view detaches existing UI.
// The App owns platform UI resources; the caller retains its View and nodes.
func Attach(app *engine.App, view *ui.View) error {
	if view == nil {
		return app.SetUI(nil)
	}

	return app.SetUI(&attachment{View: view})
}

// View returns the retained View attached to app, or nil when none is attached.
func View(app *engine.App) *ui.View {
	attached, ok := app.UI().(*attachment)
	if !ok {
		return nil
	}

	return attached.View
}
