package engine

import (
	"time"

	"github.com/karty-game/ashura/internal/platform"
	"github.com/karty-game/ashura/internal/render"
	"github.com/karty-game/ashura/internal/uihost"
)

// UI is the opaque retained-interface attachment installed by pkg/ui/app.
// Applications construct and retrieve their concrete View through that package.
type UI = uihost.Attachment

// UI returns the optional interface attachment, or nil when none is configured.
// Like other App methods, call it before Run or on the App loop goroutine.
func (app *App) UI() UI {
	if app == nil {
		return nil
	}

	return app.ui
}

// SetUI installs the attachment created by pkg/ui/app before Run.
// Use uiapp.Attach for a retained View, or pass nil to detach it before Run.
func (app *App) SetUI(view UI) error {
	if app == nil || app.closed {
		return errAppClosed
	}

	if app.driver != nil {
		return errAppRunning
	}

	app.ui = view

	return nil
}
func (app *App) filterUI(input platform.InputState) platform.InputState {
	if app.ui == nil {
		return input
	}

	if app.ui.CapturesKeyboard() {
		clear(input.Keys[:])
	}

	captured := app.ui.CapturesPointer()
	if captured || app.ui.ConsumesPointer(input.MouseX, input.MouseY) {
		clear(input.MouseButtons[:])
		clear(input.DragDX[:])
		clear(input.DragDY[:])
		input.MouseDX, input.MouseDY = 0, 0
		input.WheelX, input.WheelY = 0, 0
	}

	touch := input.Touch
	if captured || (touch.Down || touch.Pressed) && app.ui.ConsumesPointer(touch.X, touch.Y) {
		input.Touch = platform.TouchPointer{}
		input.TouchDX, input.TouchDY = 0, 0
		input.TouchPanX, input.TouchPanY, input.Pinch = 0, 0, 0
	}

	return input
}

func (app *App) composeUI(frame *render.Frame, input platform.InputState, deltaTime time.Duration, width, height int) error {
	if app.ui == nil {
		return nil
	}

	logicalWidth, logicalHeight := input.LogicalWidth, input.LogicalHeight
	if logicalWidth <= 0 {
		logicalWidth = float32(width)
	}

	if logicalHeight <= 0 {
		logicalHeight = float32(height)
	}

	if err := app.ui.Resize(logicalWidth, logicalHeight); err != nil {
		return err
	}

	app.uiTime += deltaTime

	var uiErr error

	frame.UI, uiErr = app.ui.Frame(app.uiTime)
	if uiErr != nil {
		return uiErr
	}

	if app.ui.CapturesKeyboard() || app.ui.CapturesPointer() || app.ui.PreventsPointerLock() {
		frame.PointerLock = false
	}

	return nil
}
