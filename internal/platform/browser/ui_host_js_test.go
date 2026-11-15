//go:build js && wasm

package browser

import (
	"errors"
	"syscall/js"
	"testing"

	"github.com/karty-game/karty-webgpu/internal/uipaint"
	"github.com/karty-game/karty-webgpu/pkg/ui"
)

func TestUISubmissionRequiresAttachedBackend(t *testing.T) {
	t.Parallel()

	driver := &Driver{}
	if err := driver.updateAttachedUI(nil); err != nil {
		t.Fatal("scene without UI requires an optional backend", err)
	}

	if err := driver.updateAttachedUI(&uipaint.Frame{}); !errors.Is(err, errDrawBuffer) {
		t.Fatal("UI submission without its attachment was silently discarded", err)
	}
}

func TestUIAttachmentRejectsInvalidOrOccupiedHosts(t *testing.T) {
	t.Parallel()

	view := &ui.View{}
	occupied := &Driver{}
	previous := &browserUIBridge{Driver: occupied, ui: view}
	occupied.uiBridge = previous

	for _, host := range []any{nil, "foreign host", (*Driver)(nil), &Driver{}, &Driver{closed: true}, &Driver{running: true}, occupied} {
		if bridge, err := AttachUI(host, view); bridge != nil || !errors.Is(err, errUIHost) {
			t.Fatalf("invalid/occupied host accepted: bridge=%v err=%v", bridge, err)
		}
	}

	if occupied.uiBridge != previous {
		t.Fatal("duplicate attachment replaced the existing UI owner")
	}
}

//nolint:paralleltest // The fixture may own the Node document shim.
func TestFailedUIAttachmentRemainsOwnedUntilDriverClose(t *testing.T) {
	factory := js.Global().Get("Function").New(`return {
  destroyed: 0,
  createShaderModule() { return {}; },
  createRenderPipelineAsync() { return Promise.reject(new Error("fixture pipeline rejection")); },
  destroy() { this.destroyed++; }
 }`)
	device := factory.Invoke()
	canvas := js.Global().Get("Object").New()
	driver := &Driver{device: device, canvas: canvas}
	view := &ui.View{}

	bridge, err := AttachUI(driver, view)
	if !errors.Is(err, errPromiseRejected) || bridge == nil || driver.uiBridge == nil {
		t.Fatalf("failed preparation lost its cleanup owner: bridge=%v err=%v", bridge, err)
	}

	if len(driver.listeners) != 0 {
		t.Fatal("failed GPU preparation installed UI event listeners")
	}

	document := js.Global().Get("document")
	if document.Type() != js.TypeObject {
		shim := js.Global().Get("Object").New()
		shim.Set("pointerLockElement", js.Null())
		js.Global().Set("document", shim)
		t.Cleanup(func() { js.Global().Set("document", document) })
	}

	if err := driver.Close(); err != nil {
		t.Fatal(err)
	}

	if driver.uiBridge != nil || device.Get("destroyed").Int() != 1 {
		t.Fatal("driver cleanup retained failed UI resources or skipped its device")
	}

	if err := driver.Close(); err != nil || device.Get("destroyed").Int() != 1 {
		t.Fatal("repeated close released the device again", err)
	}
}
