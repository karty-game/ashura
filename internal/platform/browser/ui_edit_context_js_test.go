//go:build js && wasm

package browser

import (
	"syscall/js"
	"testing"

	"github.com/karty-game/ashura/pkg/ui"
	"github.com/karty-game/ashura/pkg/ui/fonts/defaultfont"
)

//nolint:paralleltest // The fixture owns real browser document focus and EditContext attachment.
func TestEditContextAttachesOnlyForWritableTextFocus(t *testing.T) {
	document := js.Global().Get("document")
	if document.Type() != js.TypeObject || js.Global().Get("EditContext").Type() != js.TypeFunction {
		t.Skip("requires browser DOM and EditContext")
	}

	font, err := defaultfont.Load()
	if err != nil {
		t.Fatal(err)
	}

	input := ui.NewInput("Name", "initial", "", nil)
	button := ui.NewButton("Camera", nil)

	view, err := ui.New(ui.Column(input, button), font)
	if err != nil {
		t.Fatal(err)
	}

	canvas := document.Call("createElement", "canvas")
	canvas.Set("tabIndex", 0)
	canvas.Get("style").Set("width", "640px")
	canvas.Get("style").Set("height", "480px")
	document.Get("body").Call("appendChild", canvas)
	driver := &browserUIBridge{Driver: &Driver{canvas: canvas, window: js.Global().Get("window")}, ui: view}
	driver.uiBridge = driver

	t.Cleanup(func() {
		driver.closeUIInput()
		driver.removeInputListeners()
		canvas.Call("remove")
	})
	driver.installUIInput()

	assertDetached := func() {
		t.Helper()

		if !canvas.Get("editContext").IsNull() {
			t.Fatal("canvas is editable without a writable text input")
		}
	}
	assertDetached()

	for range 3 {
		driver.syncUIInput()
		canvas.Call("focus")
		assertDetached()
	}

	view.SetFocus(button)
	driver.syncUIInput()
	assertDetached()

	view.SetFocus(input)
	driver.syncUIInput()

	if !canvas.Get("editContext").Equal(driver.uiInput.edit) || driver.uiInput.edit.Get("text").String() != "initial" {
		t.Fatal("writable text focus did not activate and initialize EditContext")
	}

	input.SetReadOnly(true)
	driver.syncUIInput()
	assertDetached()

	input.SetReadOnly(false)
	driver.syncUIInput()

	if !canvas.Get("editContext").Equal(driver.uiInput.edit) {
		t.Fatal("writable text focus did not reactivate EditContext")
	}

	view.SetFocus(button)
	driver.syncUIInput()
	assertDetached()
}
