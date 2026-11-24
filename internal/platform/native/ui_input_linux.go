//go:build linux

//nolint:mnd // SDL contact identity reserves distinct mouse and touch namespaces.
package native

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Zyko0/go-sdl3/sdl"
	ui "github.com/karty-game/ashura/internal/uihost"
)

type nativeUI struct {
	cursor         nativeUICursor
	captured       bool
	capturePointer int64
	editing        ui.ID
	composing      bool
	start, end     int
	area           ui.Rect
}

//nolint:cyclop,gosec,funlen // Independent SDL event families; native timestamps/contact bit patterns map to signed IDs/durations.
func (driver *nativeUIHost) readUIEvent(event *sdl.Event) bool {
	if driver.ui == nil {
		return false
	}

	normalized := ui.Event{Pointer: 1, Device: "mouse"}
	//nolint:exhaustive // Only UI-related SDL event families are handled here.
	switch event.Type {
	case sdl.EVENT_MOUSE_BUTTON_DOWN, sdl.EVENT_MOUSE_BUTTON_UP:
		button := event.MouseButtonEvent()
		normalized.X, normalized.Y = button.X, button.Y
		normalized.Button = int(button.Button) - 1
		normalized.Time = time.Duration(button.Timestamp)

		normalized.Kind = ui.PointerUp
		if button.Down {
			normalized.Kind = ui.PointerDown
		}
	case sdl.EVENT_MOUSE_MOTION:
		m := event.MouseMotionEvent()
		normalized.Kind = ui.PointerMove
		normalized.X, normalized.Y = m.X, m.Y
		normalized.Time = time.Duration(m.Timestamp)
	case sdl.EVENT_MOUSE_WHEEL:
		wheel := event.MouseWheelEvent()

		direction := float32(40)
		if wheel.Direction == sdl.MOUSEWHEEL_FLIPPED {
			direction = -direction
		}

		normalized.Kind = ui.Wheel
		normalized.X, normalized.Y = wheel.MouseX, wheel.MouseY
		normalized.DeltaX, normalized.DeltaY = wheel.X*direction, -wheel.Y*direction
	case sdl.EVENT_KEY_DOWN, sdl.EVENT_KEY_UP:
		keyEvent := event.KeyboardEvent()
		normalized.Key = nativeUIKey(keyEvent.Key.KeyName())

		normalized.Kind = ui.KeyUp
		if keyEvent.Down {
			normalized.Kind = ui.KeyDown
		}

		normalized.Repeat = keyEvent.Repeat
		normalized.Time = time.Duration(keyEvent.Timestamp)
		// SDL Keymod bit layout: left/right Shift, Ctrl, Alt and GUI.
		normalized.Modifiers = ui.Modifiers{
			Shift:   keyEvent.Mod&0x0003 != 0,
			Control: keyEvent.Mod&0x00c0 != 0,
			Alt:     keyEvent.Mod&0x0300 != 0,
			Meta:    keyEvent.Mod&0x0c00 != 0,
		}
	case sdl.EVENT_TEXT_INPUT:
		normalized.Kind = ui.InsertText

		normalized.Text = event.TextInputEvent().Text
		if driver.uiInput.composing {
			normalized.Kind = ui.ReplaceText
			normalized.Start, normalized.End = driver.uiInput.start, driver.uiInput.end
			normalized.Anchor, normalized.Cursor = normalized.Start+len(normalized.Text), normalized.Start+len(normalized.Text)
			driver.ui.Dispatch(normalized)
			driver.ui.Dispatch(ui.Event{Kind: ui.CompositionEnd})
			driver.uiInput.composing = false
			driver.syncUIInput()

			return true
		}
	case sdl.EVENT_TEXT_EDITING:
		driver.nativeComposition(event.TextEditingEvent())

		return true
	case sdl.EVENT_WINDOW_MOUSE_LEAVE:
		driver.uiInput.cursor.active = false

		driver.uiInput.cursor.dirty = true
		if !driver.ui.CapturesPointer() {
			driver.ui.Dispatch(ui.Event{Kind: ui.PointerMove, X: -1, Y: -1})
		}

		driver.syncUICursor()

		return false
	case sdl.EVENT_WINDOW_FOCUS_LOST:
		driver.uiInput.cursor.active = false
		driver.uiInput.cursor.dirty = true
		normalized.Kind = ui.Blur
	case sdl.EVENT_FINGER_DOWN, sdl.EVENT_FINGER_MOTION, sdl.EVENT_FINGER_UP, sdl.EVENT_FINGER_CANCELED:
		f := event.TouchFingerEvent()
		normalized.Device = "touch"
		normalized.Pointer = int64(f.FingerID) + 2
		normalized.X, normalized.Y = f.X*driver.navigation.width, f.Y*driver.navigation.height
		//nolint:exhaustive // Only UI-related SDL event families are handled here.
		switch event.Type {
		case sdl.EVENT_FINGER_DOWN:
			normalized.Kind = ui.PointerDown
		case sdl.EVENT_FINGER_MOTION:
			normalized.Kind = ui.PointerMove
		case sdl.EVENT_FINGER_UP:
			normalized.Kind = ui.PointerUp
		default:
			normalized.Kind = ui.PointerCancel
		}
	default:
		return false
	}

	if normalized.Device == "mouse" &&
		(normalized.Kind == ui.PointerMove || normalized.Kind == ui.PointerDown || normalized.Kind == ui.PointerUp) {
		driver.uiInput.cursor.x, driver.uiInput.cursor.y = normalized.X, normalized.Y
		driver.uiInput.cursor.active = true
		driver.uiInput.cursor.dirty = true
	}

	result := driver.ui.Dispatch(normalized)
	if result.Capture && normalized.Device != "touch" {
		if err := sdl.CaptureMouse(true); err != nil {
			driver.ui.Dispatch(ui.Event{Kind: ui.CaptureLost, Pointer: result.Pointer})
		} else {
			driver.uiInput.captured = true
			driver.uiInput.capturePointer = result.Pointer
		}
	}

	if result.Release {
		driver.releaseUICapture()
	}

	if result.ClipboardWrite {
		_ = sdl.SetClipboardText(result.Copy)
	}

	if result.ClipboardRead {
		if text, err := sdl.GetClipboardText(); err == nil {
			driver.ui.Dispatch(ui.Event{Kind: ui.InsertText, Text: text, Session: result.Session, Revision: result.Revision})
		}
	}

	driver.syncUIInput()

	return result.Consumed
}
func (driver *nativeUIHost) syncUIInput() {
	if driver.ui == nil {
		return
	}

	driver.syncUICursor()

	pointer, captured := driver.ui.CapturedPointer()
	if driver.uiInput.captured && (!captured || pointer != driver.uiInput.capturePointer) {
		driver.releaseUICapture()
	}

	editing := driver.ui.Editing()
	if !editing.Active {
		driver.uiInput.composing = false
		if driver.uiInput.editing != 0 {
			_ = driver.window.StopTextInput()
			driver.uiInput.editing = 0
		}

		return
	}

	if driver.uiInput.editing != editing.Session {
		driver.uiInput.composing = false
		if err := driver.window.StartTextInput(); err != nil {
			caps := driver.ui.Capabilities()
			caps.TextInput = false
			caps.Composition = false
			driver.ui.SetCapabilities(caps)

			return
		}

		driver.uiInput.editing = editing.Session
	}

	if driver.uiInput.area != editing.Caret {
		r := editing.Caret
		_ = driver.window.SetTextInputArea(&sdl.Rect{X: int32(r.X), Y: int32(r.Y), W: max(1, int32(r.W)), H: max(1, int32(r.H))}, 0)
		driver.uiInput.area = r
	}
}
func (driver *nativeUIHost) nativeComposition(event *sdl.TextEditingEvent) {
	state := driver.ui.Editing()
	if !state.Active {
		return
	}

	bridge := &driver.uiInput
	if event.Text == "" {
		if bridge.composing {
			driver.ui.Dispatch(ui.Event{Kind: ui.CompositionCancel})

			bridge.composing = false
		}

		return
	}

	if !bridge.composing {
		driver.ui.Dispatch(ui.Event{Kind: ui.CompositionStart})

		bridge.start, bridge.end = min(state.Anchor, state.Cursor), max(state.Anchor, state.Cursor)
		bridge.composing = true
	}

	anchor := bridge.start + runeBytes(event.Text, max(0, int(event.Start)))
	cursor := bridge.start + runeBytes(event.Text, max(0, int(event.Start+event.Length)))
	driver.ui.Dispatch(
		ui.Event{Kind: ui.ReplaceText, Start: bridge.start, End: bridge.end, Text: event.Text, Anchor: anchor, Cursor: cursor},
	)
	bridge.end = bridge.start + len(event.Text)
}
func runeBytes(text string, count int) int {
	index := 0
	for count > 0 && index < len(text) {
		_, size := utf8.DecodeRuneInString(text[index:])
		index += size
		count--
	}

	return index
}
func nativeUIKey(name string) string {
	switch name {
	case "Return", "Keypad Enter":
		return "Enter"
	case "Space":
		return " "
	case "Left", "Right", "Up", "Down":
		return "Arrow" + name
	case "PageUp":
		return "PageUp"
	case "PageDown":
		return "PageDown"
	}

	if len(name) == 1 {
		return strings.ToLower(name)
	}

	return name
}

func (driver *nativeUIHost) releaseUICapture() {
	if driver.uiInput.captured {
		_ = sdl.CaptureMouse(false)
		driver.uiInput.captured = false
	}
}
