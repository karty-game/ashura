//go:build linux

package native

import (
	"github.com/Zyko0/go-sdl3/sdl"
	ui "github.com/karty-game/karty-webgpu/internal/uihost"
)

type nativeUICursor struct {
	active, dirty, initialized bool
	x, y                       float32
	current                    ui.Cursor
	stats                      ui.Stats
	previous                   *sdl.Cursor
	cursors                    map[ui.Cursor]*sdl.Cursor
}

func (driver *nativeUIHost) syncUICursor() {
	state := &driver.uiInput.cursor

	stats := driver.ui.Stats()
	if state.initialized && !state.dirty && state.stats == stats {
		return
	}

	if !state.initialized {
		state.previous = sdl.GetCursor()
		state.cursors = make(map[ui.Cursor]*sdl.Cursor)
		state.initialized = true
	}

	state.dirty = false
	state.stats = stats

	cursor := ui.DefaultCursor
	if _, captured := driver.ui.CapturedPointer(); state.active || captured {
		cursor = driver.ui.CursorAt(state.x, state.y)
	}

	if state.current == cursor {
		return
	}

	nativeCursor := state.cursors[cursor]
	if nativeCursor == nil {
		var err error

		nativeCursor, err = sdl.CreateSystemCursor(nativeCursorKind(cursor))
		if err != nil {
			return
		}

		state.cursors[cursor] = nativeCursor
	}

	if sdl.SetCursor(nativeCursor) == nil {
		state.current = cursor
	}
}

func nativeCursorKind(cursor ui.Cursor) sdl.SystemCursor {
	switch cursor {
	case ui.DefaultCursor:
		return sdl.SYSTEM_CURSOR_DEFAULT
	case ui.PointerCursor:
		return sdl.SYSTEM_CURSOR_POINTER
	case ui.TextCursor:
		return sdl.SYSTEM_CURSOR_TEXT
	case ui.HorizontalResizeCursor:
		return sdl.SYSTEM_CURSOR_EW_RESIZE
	case ui.VerticalResizeCursor:
		return sdl.SYSTEM_CURSOR_NS_RESIZE
	default:
		return sdl.SYSTEM_CURSOR_DEFAULT
	}
}

func (driver *nativeUIHost) closeUICursor() {
	state := &driver.uiInput.cursor
	if !state.initialized {
		return
	}

	_ = sdl.SetCursor(state.previous)
	for _, cursor := range state.cursors {
		cursor.Destroy()
	}

	state.cursors = nil
	state.initialized = false
}
