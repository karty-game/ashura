//go:build linux

package native

import (
	"errors"

	"github.com/Zyko0/go-sdl3/sdl"
	"github.com/karty-game/ashura/internal/uihost"
	"github.com/karty-game/ashura/internal/uipaint"
	wgpu "github.com/karty-game/ashura/internal/webgpu"
)

var errUIAttachment = errors.New("native UI attachment requires an available native driver and a non-nil view")

// The core driver only calls these hooks. Optional UI construction is reachable
// through the frontend's explicit attachment, independently of the scene backend.
type nativeUIBridge interface {
	Resize(width, height float32)
	ReadEvent(event *sdl.Event) bool
	SyncInput()
	Update(frame *uipaint.Frame) error
	Draw(encoder *wgpu.CommandEncoder, view *wgpu.TextureView, frame *uipaint.Frame) error
	Close()
}

type nativeUIHost struct {
	*Driver

	ui      uihost.View
	uiGPU   *uiGPU
	uiInput nativeUI
	closed  bool
}

// AttachUI prepares the optional retained UI after the native device is ready.
// The driver owns the returned bridge and closes it before its GPU/window.
func AttachUI(host any, view uihost.View) (uihost.Bridge, error) {
	driver, ok := host.(*Driver)
	if !ok || driver == nil || driver.closed || driver.device == nil || driver.window == nil || driver.uiBridge != nil || view == nil {
		return nil, errUIAttachment
	}

	bridge := &nativeUIHost{Driver: driver, ui: view}

	driver.uiBridge = bridge
	if err := bridge.prepareUI(); err != nil {
		return nil, err
	}

	view.SetCapabilities(uihost.Capabilities{TextInput: true, Composition: true, Clipboard: true, Capture: true})

	width, height, err := driver.window.Size()
	if err != nil {
		return nil, err
	}

	if err := view.Resize(float32(width), float32(height)); err != nil {
		return nil, err
	}

	return bridge, nil
}

func (driver *Driver) updateAttachedUI(frame *uipaint.Frame) error {
	if driver.uiBridge != nil {
		return driver.uiBridge.Update(frame)
	}

	if frame != nil {
		return errEmptySurfaceFrame
	}

	return nil
}

func (bridge *nativeUIHost) Resize(width, height float32) { _ = bridge.ui.Resize(width, height) }

func (bridge *nativeUIHost) ReadEvent(event *sdl.Event) bool { return bridge.readUIEvent(event) }

func (bridge *nativeUIHost) SyncInput() { bridge.syncUIInput() }

func (bridge *nativeUIHost) Update(frame *uipaint.Frame) error { return bridge.updateUI(frame) }

func (bridge *nativeUIHost) Draw(encoder *wgpu.CommandEncoder, view *wgpu.TextureView, frame *uipaint.Frame) error {
	return bridge.drawUI(encoder, view, frame)
}

func (bridge *nativeUIHost) Close() {
	if bridge.closed {
		return
	}

	bridge.closed = true
	bridge.closeUI()
	bridge.closeUICursor()
	bridge.releaseUICapture()
}
