//go:build js && wasm

package browser

import (
	"errors"
	"syscall/js"

	"github.com/karty-game/ashura/internal/uihost"
	"github.com/karty-game/ashura/internal/uipaint"
)

// uiBackend is installed only by the optional UI attachment. Keeping its
// implementation behind this boundary leaves input, rendering and text support
// unreachable when an application does not attach a UI.
type uiBackend interface {
	resize(width, height float32)
	readKey(event js.Value, down bool) bool
	consumesPointer(event js.Value) bool
	consumesPointerAt(x, y float32) bool
	capturesPointer() bool
	syncInput()
	update(frame *uipaint.Frame) error
	draw(encoder, view js.Value, frame *uipaint.Frame)
	Close()
}

type browserUIBridge struct {
	*Driver

	ui      uihost.View
	uiGPU   *uiGPU
	uiInput browserUI
}

var errUIHost = errors.New("browser: invalid or already attached UI host")

// AttachUI installs the optional browser UI backend before the application loop.
// The application UI adapter owns this entry point; core driver setup never
// references the concrete UI implementation.
func AttachUI(host any, view uihost.View) (uihost.Bridge, error) {
	driver, ok := host.(*Driver)
	if !ok || driver == nil || driver.closed || driver.running || driver.uiBridge != nil || view == nil {
		return nil, errUIHost
	}

	if driver.device.IsUndefined() || driver.device.IsNull() || driver.canvas.IsUndefined() || driver.canvas.IsNull() {
		return nil, errUIHost
	}

	bridge := &browserUIBridge{Driver: driver, ui: view}

	driver.uiBridge = bridge
	if err := bridge.prepareUI(); err != nil {
		return bridge, err
	}

	bridge.installUIInput()

	return bridge, nil
}

func (bridge *browserUIBridge) Close() {
	bridge.closeUI()
	bridge.closeUIInput()
	bridge.uiInput.installed = false
	bridge.uiBridge = nil
}

func (bridge *browserUIBridge) resize(width, height float32) {
	_ = bridge.ui.Resize(width, height)
}

func (bridge *browserUIBridge) readKey(event js.Value, down bool) bool {
	return bridge.readUIKey(event, down)
}

func (bridge *browserUIBridge) consumesPointer(event js.Value) bool {
	x, y := bridge.uiPoint(event)

	return bridge.consumesPointerAt(x, y) || bridge.ui.CapturesKeyboard()
}

func (bridge *browserUIBridge) consumesPointerAt(x, y float32) bool {
	return bridge.ui.ConsumesPointer(x, y)
}

func (bridge *browserUIBridge) capturesPointer() bool { return bridge.ui.CapturesPointer() }

func (bridge *browserUIBridge) syncInput() { bridge.syncUIInput() }

func (bridge *browserUIBridge) update(frame *uipaint.Frame) error { return bridge.updateUI(frame) }

func (bridge *browserUIBridge) draw(encoder, view js.Value, frame *uipaint.Frame) {
	bridge.drawUI(encoder, view, frame)
}
