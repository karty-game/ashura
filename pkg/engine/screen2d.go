package engine

import (
	"github.com/karty-game/karty-webgpu/internal/spritepaint"
	"github.com/karty-game/karty-webgpu/pkg/draw2d"
)

type screenLayer interface {
	resize(width, height int)
	Frame(width, height int) *spritepaint.Frame
	Stats() draw2d.Stats
	Close()
}

type screenAttachment struct{ *draw2d.Screen }

func (screen *screenAttachment) resize(width, height int) { screen.Resize(width, height) }

// ViewportSize returns framebuffer pixels, falling back to the configured size
// before the first frame. It does not create a sprite layer or UI.
func (app *App) ViewportSize() (int, int) {
	if app == nil {
		return 0, 0
	}

	return app.frameSize(app.input.Width, app.input.Height)
}

// Screen2D returns the App-owned pixel-space sprite layer between 3D and UI.
// The layer is created on first use and lives until the App closes. All changes
// belong to the application update goroutine. Calling it after Close panics.
func (app *App) Screen2D() *draw2d.Screen {
	if app == nil || app.closed {
		panic("engine: Screen2D on closed App")
	}

	if app.screen2D == nil {
		app.screen2D = &screenAttachment{Screen: draw2d.NewScreen()}
		width, height := app.frameSize(app.input.Width, app.input.Height)
		app.screen2D.resize(width, height)
	}

	attached, ok := app.screen2D.(*screenAttachment)
	if !ok {
		panic("engine: invalid sprite layer attachment")
	}

	return attached.Screen
}

func (app *App) frameSize(width, height int) (int, int) {
	if width <= 0 {
		width = app.config.Width
	}

	if height <= 0 {
		height = app.config.Height
	}

	return width, height
}
