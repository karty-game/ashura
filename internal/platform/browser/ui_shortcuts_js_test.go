//go:build js && wasm

package browser

import (
	"syscall/js"
	"testing"

	"github.com/karty-game/ashura/pkg/ui"
	"github.com/karty-game/ashura/pkg/ui/fonts/defaultfont"
)

//nolint:paralleltest // The test owns document focus or the Node document shim.
func TestUnfocusedKeysReachOnlyGlobalUIPreviews(t *testing.T) {
	canvas := unfocusedShortcutCanvas(t)

	font, err := defaultfont.Load()
	if err != nil {
		t.Fatal(err)
	}

	input := ui.NewInput("Name", "unchanged", "", nil)

	view, err := ui.New(ui.Column(input, ui.NewButton("Next", nil)), font)
	if err != nil {
		t.Fatal(err)
	}

	view.SetFocus(input)

	shortcuts := 0

	view.OnPreview(func(event *ui.RoutedEvent) {
		if event.Event.Kind == ui.KeyDown && event.Event.Key == "F3" {
			shortcuts++

			event.PreventDefault()
		}
	})
	driver := &browserUIBridge{Driver: &Driver{canvas: canvas}, ui: view}
	prevented := 0

	prevent := js.FuncOf(func(js.Value, []js.Value) any {
		prevented++

		return nil
	})
	defer prevent.Release()

	for _, key := range []string{"z", "Tab", "F3"} {
		event := js.ValueOf(map[string]any{
			"key": key, "code": key, "timeStamp": 1, "cancelable": true,
			"repeat": false, "shiftKey": false, "ctrlKey": false, "altKey": false, "metaKey": false,
			"preventDefault": prevent,
		})
		if handled := driver.readUIKey(event, true); handled != (key == "F3") {
			t.Fatalf("foreign-focus key %s handled=%t", key, handled)
		}
	}

	if shortcuts != 1 || prevented != 1 || input.Text() != "unchanged" || view.Focused() != input {
		t.Fatalf("global shortcut changed editing/focus or default handling: shortcuts=%d prevented=%d", shortcuts, prevented)
	}
}

func unfocusedShortcutCanvas(t *testing.T) js.Value {
	t.Helper()

	document := js.Global().Get("document")
	if document.Type() != js.TypeObject {
		canvas := js.Global().Get("Object").New()
		foreign := js.Global().Get("Object").New()
		shim := js.Global().Get("Object").New()
		shim.Set("activeElement", foreign)
		js.Global().Set("document", shim)
		t.Cleanup(func() { js.Global().Set("document", document) })

		return canvas
	}

	canvas := document.Call("createElement", "canvas")
	foreign := document.Call("createElement", "input")
	foreign.Get("style").Set("position", "fixed")
	foreign.Get("style").Set("left", "-2000px")
	document.Get("body").Call("appendChild", canvas)
	document.Get("body").Call("appendChild", foreign)
	foreign.Call("focus")
	t.Cleanup(func() { foreign.Call("remove"); canvas.Call("remove") })

	return canvas
}
