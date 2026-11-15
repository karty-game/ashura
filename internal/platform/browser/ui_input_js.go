//go:build js && wasm

//nolint:mnd // DOM wheel modes and UTF-16 widths are platform ABI constants.
package browser

import (
	"syscall/js"
	"time"
	"unicode/utf16"

	ui "github.com/karty-game/karty-webgpu/internal/uihost"
)

type browserUI struct {
	installed          bool
	oldOutline         string
	oldCursor          string
	cursor             ui.Cursor
	cursorStats        ui.Stats
	cursorDirty        bool
	pointerActive      bool
	pointerX, pointerY float32
	capturePointer     int64
	captured           bool
	lastState          ui.EditingState
	edit               js.Value
	session            ui.ID
	composing          bool
	oldTouch           string
	lastRevision       uint64
}

func (driver *browserUIBridge) installUIInput() {
	if driver.ui == nil {
		return
	}

	driver.uiInput.installed = true
	driver.uiInput.oldCursor = driver.canvas.Get("style").Get("cursor").String()
	driver.uiInput.cursorDirty = true
	driver.uiInput.oldOutline = driver.canvas.Get("style").Get("outline").String()
	driver.canvas.Get("style").Set("outline", "none")
	driver.uiInput.oldTouch = driver.canvas.Get("style").Get("touchAction").String()
	driver.canvas.Get("style").Set("touchAction", "none")

	context := js.Global().Get("EditContext")
	if context.Type() == js.TypeFunction {
		driver.uiInput.edit = context.New()
		// Attach only when Editing reports an active writable input. Attaching
		// here makes an otherwise non-editable canvas summon a mobile keyboard.
		driver.listen(driver.uiInput.edit, "textupdate", driver.uiTextUpdate)
		driver.listen(driver.uiInput.edit, "compositionstart", func(js.Value) {
			driver.uiInput.composing = true
			driver.ui.Dispatch(ui.Event{Kind: ui.CompositionStart})
		})
		driver.listen(driver.uiInput.edit, "compositionend", func(js.Value) {
			driver.uiInput.composing = false
			driver.ui.Dispatch(ui.Event{Kind: ui.CompositionEnd})
			driver.syncUIInput()
		})
		driver.listen(driver.uiInput.edit, "characterboundsupdate", func(e js.Value) { driver.uiCharacterBounds(e) })
	}

	driver.ui.SetCapabilities(
		ui.Capabilities{
			TextInput:   context.Type() == js.TypeFunction,
			Composition: context.Type() == js.TypeFunction,
			Clipboard:   !js.Global().Get("navigator").Get("clipboard").IsUndefined(),
			Capture:     true,
		},
	)
	driver.resizeCanvas()

	for name, kind := range map[string]ui.EventKind{"pointerdown": ui.PointerDown, "pointermove": ui.PointerMove, "pointerup": ui.PointerUp, "pointercancel": ui.PointerCancel, "lostpointercapture": ui.CaptureLost} {
		target := driver.canvas
		if kind != ui.PointerDown {
			target = js.Global().Get("window")
		}

		driver.listen(target, name, func(e js.Value) { driver.uiPointer(e, kind) })
	}

	driver.listen(driver.canvas, "pointerleave", driver.uiPointerLeave)

	driver.installUIWheel()
	driver.installUIMotion()

	driver.listen(js.Global().Get("window"), "blur", func(js.Value) {
		driver.uiInput.pointerActive = false
		driver.uiInput.cursorDirty = true
		driver.applyUIResult(driver.ui.Dispatch(ui.Event{Kind: ui.Blur}))
		driver.syncUIInput()
	})
	driver.listen(driver.canvas, "click", func(e js.Value) {
		if e.Get("detail").Int() == 0 {
			if target := driver.ui.FocusedID(); target != 0 {
				driver.ui.Dispatch(ui.Event{Kind: ui.Activate, Target: target})
			}
		}
	})
}

func (driver *browserUIBridge) uiPoint(event js.Value) (float32, float32) {
	bounds := driver.canvas.Call("getBoundingClientRect")
	pointX := float32(event.Get("clientX").Float() - bounds.Get("left").Float())
	pointY := float32(event.Get("clientY").Float() - bounds.Get("top").Float())

	return pointX, pointY
}
func (driver *browserUIBridge) uiPointer(event js.Value, kind ui.EventKind) {
	if driver.ui == nil {
		return
	}

	pointX, pointY := driver.uiPoint(event)
	if event.Get("pointerType").String() != "touch" {
		driver.uiInput.pointerX, driver.uiInput.pointerY = pointX, pointY
		driver.uiInput.pointerActive = true
		driver.uiInput.cursorDirty = true
	}

	button := event.Get("button").Int()
	if kind == ui.PointerMove {
		button = 0
	}

	if kind == ui.PointerDown {
		driver.canvas.Call("focus", jsObject(map[string]any{"preventScroll": true}))
	}

	result := driver.ui.Dispatch(
		ui.Event{
			Kind:      kind,
			Pointer:   int64(event.Get("pointerId").Int()),
			Device:    event.Get("pointerType").String(),
			X:         pointX,
			Y:         pointY,
			Button:    button,
			Modifiers: browserUIModifiers(event),
			Time:      time.Duration(event.Get("timeStamp").Float() * float64(time.Millisecond)),
		},
	)
	driver.applyUIResult(result)

	if result.Consumed {
		driver.resetInputDeltas()
		clear(driver.input.MouseButtons[:])

		if event.Get("cancelable").Truthy() {
			event.Call("preventDefault")
		}
	}

	driver.syncUIInput()
}
func (driver *browserUIBridge) applyUIResult(result ui.Result) {
	if result.Capture {
		driver.uiCapture(result.Pointer)
	}

	if result.Release {
		driver.releaseUICapture()
	}

	if result.ClipboardWrite {
		clipboard := js.Global().Get("navigator").Get("clipboard")
		if !clipboard.IsUndefined() {
			driver.uiPromise(clipboard.Call("writeText", result.Copy), nil)
		}
	}

	if result.ClipboardRead {
		clipboard := js.Global().Get("navigator").Get("clipboard")
		if !clipboard.IsUndefined() {
			driver.uiPromise(clipboard.Call("readText"), func(text string) {
				driver.ui.Dispatch(ui.Event{Kind: ui.InsertText, Text: text, Session: result.Session, Revision: result.Revision})
				driver.syncUIInput()
			})
		}
	}
}
func (driver *browserUIBridge) uiCapture(pointer int64) {
	defer func() {
		if recover() != nil {
			driver.ui.Dispatch(ui.Event{Kind: ui.CaptureLost, Pointer: pointer})
		}
	}()

	driver.canvas.Call("setPointerCapture", float64(pointer))
	driver.uiInput.capturePointer, driver.uiInput.captured = pointer, true
}
func (driver *browserUIBridge) uiPromise(promise js.Value, accept func(string)) {
	var success, failure js.Func

	success = js.FuncOf(func(_ js.Value, args []js.Value) any {
		defer success.Release()
		defer failure.Release()

		if !driver.closed && accept != nil && len(args) > 0 {
			accept(args[0].String())
		}

		return nil
	})
	failure = js.FuncOf(func(js.Value, []js.Value) any {
		defer success.Release()
		defer failure.Release()

		return nil
	})
	promise.Call("then", success, failure)
}
func browserUIModifiers(e js.Value) ui.Modifiers {
	return ui.Modifiers{
		Shift:   e.Get("shiftKey").Truthy(),
		Control: e.Get("ctrlKey").Truthy(),
		Alt:     e.Get("altKey").Truthy(),
		Meta:    e.Get("metaKey").Truthy(),
	}
}

func (driver *browserUIBridge) readUIKey(event js.Value, down bool) bool {
	if driver.ui == nil {
		return false
	}

	kind := ui.KeyUp
	if down {
		kind = ui.KeyDown
	}

	key := event.Get("key").String()

	input := ui.Event{
		Kind:      kind,
		Key:       key,
		Code:      event.Get("code").String(),
		Modifiers: browserUIModifiers(event),
		Repeat:    event.Get("repeat").Truthy(),
		Time:      time.Duration(event.Get("timeStamp").Float() * float64(time.Millisecond)),
	}
	if !js.Global().Get("document").Get("activeElement").Equal(driver.canvas) {
		result := driver.ui.DispatchPreview(input)
		driver.applyUIResult(result)

		if result.Consumed && event.Get("cancelable").Truthy() {
			event.Call("preventDefault")
		}

		return result.Consumed
	}

	editing := driver.ui.Editing()
	// EditContext supplies committed text/IME. Core owns navigation/selection and clipboard.
	if editing.Active && driver.uiInput.edit.Type() == js.TypeObject && len([]rune(key)) == 1 && !event.Get("ctrlKey").Truthy() &&
		!event.Get("metaKey").Truthy() &&
		!event.Get("altKey").Truthy() {
		return true
	}

	result := driver.ui.Dispatch(input)
	driver.applyUIResult(result)

	if result.Consumed && event.Get("cancelable").Truthy() {
		event.Call("preventDefault")
	}

	driver.syncUIInput()

	return result.Consumed || editing.Active
}

func (driver *browserUIBridge) uiTextUpdate(event js.Value) {
	state := driver.ui.Editing()
	if !state.Active {
		return
	}

	text := event.Get("text").String()
	start := utf16Byte(state.Text, event.Get("updateRangeStart").Int())

	end := utf16Byte(state.Text, event.Get("updateRangeEnd").Int())
	if start > end {
		return
	}

	updated := state.Text[:start] + text + state.Text[end:]
	driver.ui.Dispatch(
		ui.Event{
			Kind:    ui.ReplaceText,
			Session: state.Session,
			Start:   start,
			End:     end,
			Text:    text,
			Anchor:  utf16Byte(updated, event.Get("selectionStart").Int()),
			Cursor:  utf16Byte(updated, event.Get("selectionEnd").Int()),
		},
	)
	driver.syncUIInput()
}
func (driver *browserUIBridge) syncUIInput() {
	if driver.ui == nil {
		return
	}

	driver.syncUICursor()

	pointer, captured := driver.ui.CapturedPointer()
	if driver.uiInput.captured && (!captured || pointer != driver.uiInput.capturePointer) {
		driver.releaseUICapture()
	}

	if driver.uiInput.edit.Type() != js.TypeObject {
		return
	}

	state := driver.ui.Editing()
	if state == driver.uiInput.lastState {
		return
	}

	driver.uiInput.lastState = state

	edit := driver.uiInput.edit
	if !state.Active {
		driver.uiInput.session = 0
		driver.uiInput.composing = false
		driver.canvas.Set("editContext", js.Null())

		return
	}

	if !driver.canvas.Get("editContext").Equal(edit) {
		driver.canvas.Set("editContext", edit)
	}

	if driver.uiInput.composing && state.Composing {
		return
	}

	driver.uiInput.composing = state.Composing

	old := edit.Get("text").String()
	if old != state.Text {
		edit.Call("updateText", 0, len(utf16.Encode([]rune(old))), state.Text)
	}

	start, end := byteUTF16(state.Text, min(state.Anchor, state.Cursor)), byteUTF16(state.Text, max(state.Anchor, state.Cursor))
	edit.Call("updateSelection", start, end)
	bounds := driver.canvas.Call("getBoundingClientRect")
	rect := func(r ui.Rect) js.Value {
		return js.Global().
			Get("DOMRect").
			New(bounds.Get("left").Float()+float64(r.X), bounds.Get("top").Float()+float64(r.Y), float64(r.W), float64(r.H))
	}
	edit.Call("updateControlBounds", rect(state.Bounds))
	edit.Call("updateSelectionBounds", rect(state.Caret))
	driver.uiInput.session = state.Session
	driver.uiInput.lastRevision = state.Revision
}
func (driver *browserUIBridge) uiCharacterBounds(event js.Value) {
	state := driver.ui.Editing()
	if !state.Active {
		return
	}

	start, end := event.Get("rangeStart").Int(), event.Get("rangeEnd").Int()
	if start < 0 || end < start || end-start > ui.MaxTextBytes || end > byteUTF16(state.Text, len(state.Text)) {
		return
	}

	array := js.Global().Get("Array").New(end - start)

	bounds := driver.canvas.Call("getBoundingClientRect")
	for i := start; i < end; i++ {
		r := driver.ui.CharacterBounds(utf16Byte(state.Text, i))
		array.SetIndex(
			i-start,
			js.Global().
				Get("DOMRect").
				New(bounds.Get("left").Float()+float64(r.X), bounds.Get("top").Float()+float64(r.Y), float64(r.W), float64(r.H)),
		)
	}

	driver.uiInput.edit.Call("updateCharacterBounds", start, array)
}
func (driver *browserUIBridge) closeUIInput() {
	if driver.ui != nil && driver.uiInput.installed {
		driver.ui.Dispatch(ui.Event{Kind: ui.Blur})
		driver.releaseUICapture()
		driver.canvas.Get("style").Set("touchAction", driver.uiInput.oldTouch)
		driver.canvas.Get("style").Set("outline", driver.uiInput.oldOutline)
		driver.canvas.Get("style").Set("cursor", driver.uiInput.oldCursor)
		driver.canvas.Set("editContext", js.Null())
	}
}
func byteUTF16(text string, index int) int {
	index = max(0, min(len(text), index))

	return len(utf16.Encode([]rune(text[:index])))
}
func utf16Byte(text string, index int) int {
	units := 0
	for offset, character := range text {
		if units >= index {
			return offset
		}

		units++
		if character > 0xffff {
			units++
		}
	}

	return len(text)
}

func (driver *browserUIBridge) installUIWheel() {
	driver.listen(driver.canvas, "wheel", func(event js.Value) {
		pointX, pointY := driver.uiPoint(event)
		scale := float32(1)

		switch event.Get("deltaMode").Int() {
		case 1:
			scale = 20
		case 2:
			scale = driver.input.LogicalHeight
		}

		result := driver.ui.Dispatch(
			ui.Event{
				Kind:   ui.Wheel,
				X:      pointX,
				Y:      pointY,
				DeltaX: float32(event.Get("deltaX").Float()) * scale,
				DeltaY: float32(event.Get("deltaY").Float()) * scale,
			},
		)
		if result.Consumed {
			driver.input.WheelX, driver.input.WheelY = 0, 0

			event.Call("preventDefault")
		}
	})
}

func (driver *browserUIBridge) releaseUICapture() {
	if !driver.uiInput.captured {
		return
	}

	pointer := driver.uiInput.capturePointer

	driver.uiInput.captured = false
	if driver.canvas.Call("hasPointerCapture", float64(pointer)).Bool() {
		driver.canvas.Call("releasePointerCapture", float64(pointer))
	}
}
