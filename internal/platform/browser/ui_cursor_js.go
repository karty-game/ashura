//go:build js && wasm

package browser

import (
	"syscall/js"

	ui "github.com/karty-game/karty-webgpu/internal/uihost"
)

func (driver *browserUIBridge) uiPointerLeave(js.Value) {
	driver.uiInput.pointerActive = false
	driver.uiInput.cursorDirty = true

	if !driver.ui.CapturesPointer() {
		driver.ui.Dispatch(ui.Event{Kind: ui.PointerMove, X: -1, Y: -1})
	}

	driver.syncUICursor()
}

func (driver *browserUIBridge) syncUICursor() {
	state := &driver.uiInput

	stats := driver.ui.Stats()
	if !state.cursorDirty && state.cursorStats == stats {
		return
	}

	state.cursorDirty = false
	state.cursorStats = stats

	cursor := ui.DefaultCursor
	if _, captured := driver.ui.CapturedPointer(); state.pointerActive || captured {
		cursor = driver.ui.CursorAt(state.pointerX, state.pointerY)
	}

	if cursor != state.cursor {
		driver.canvas.Get("style").Set("cursor", string(cursor))
		state.cursor = cursor
	}
}
