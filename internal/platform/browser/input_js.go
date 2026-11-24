//go:build js && wasm

package browser

import (
	"syscall/js"

	"github.com/karty-game/ashura/internal/platform"
)

const touchPointerType = "touch"

func (driver *Driver) installInput() {
	window := js.Global().Get("window")
	document := js.Global().Get("document")

	driver.canvas.Set("tabIndex", 0)
	driver.installKeyboard(window, document)
	driver.installMouse(window, document)
	driver.installTouch()
	driver.listen(window, "blur", func(js.Value) {
		clear(driver.input.Keys[:])
		driver.input.MouseButtons = [8]bool{}
		driver.resetInputDeltas()
		driver.touch.Reset()
		driver.input.Touch = platform.TouchPointer{}
	})
}

func (driver *Driver) installKeyboard(window, document js.Value) {
	driver.listen(window, "keydown", func(event js.Value) {
		if driver.uiBridge != nil && driver.uiBridge.readKey(event, true) {
			if key := browserKey(event.Get(shaderCodeProperty).String()); key != 0 {
				driver.input.Keys[key] = false
			}

			return
		}

		if !document.Get("activeElement").Equal(driver.canvas) && !driver.input.PointerLocked {
			return
		}

		if key := browserKey(event.Get(shaderCodeProperty).String()); key != 0 {
			driver.input.Keys[key] = true

			event.Call("preventDefault")
		}
	})
	driver.listen(window, "keyup", func(event js.Value) {
		if key := browserKey(event.Get(shaderCodeProperty).String()); key != 0 {
			driver.input.Keys[key] = false
		}

		if driver.uiBridge != nil && driver.uiBridge.readKey(event, false) {
			return
		}

		if key := browserKey(event.Get(shaderCodeProperty).String()); key != 0 {
			driver.input.Keys[key] = false

			event.Call("preventDefault")
		}
	})
}

func (driver *Driver) installMouse(window, document js.Value) {
	driver.listen(window, "mousemove", func(event js.Value) {
		if emulatedTouchMouse(event) {
			return
		}

		bounds := driver.canvas.Call("getBoundingClientRect")
		driver.input.MouseX = float32(event.Get("clientX").Float() - bounds.Get("left").Float())
		driver.input.MouseY = float32(event.Get("clientY").Float() - bounds.Get("top").Float())
		deltaX := float32(event.Get("movementX").Float())
		deltaY := float32(event.Get("movementY").Float())
		driver.input.MouseDX += deltaX
		driver.input.MouseDY += deltaY

		for button, held := range driver.input.MouseButtons {
			if held {
				driver.input.DragDX[button] += deltaX
				driver.input.DragDY[button] += deltaY
			}
		}
	})
	driver.listen(document, "pointerlockchange", func(js.Value) {
		driver.input.PointerLocked = document.Get("pointerLockElement").Equal(driver.canvas)
		if !driver.input.PointerLocked {
			driver.input.MouseDX, driver.input.MouseDY = 0, 0
		}
	})
	driver.listen(driver.canvas, "pointerdown", func(event js.Value) {
		if event.Get("pointerType").String() == touchPointerType {
			driver.lastPointerTouch = true

			return
		}

		driver.lastPointerTouch = false
		driver.canvas.Call("focus")
		driver.input.MouseDX, driver.input.MouseDY = 0, 0
		driver.setMouseButton(event, true)
	})
	driver.listen(window, "pointerup", func(event js.Value) {
		if event.Get("pointerType").String() != touchPointerType {
			driver.setMouseButton(event, false)
		}
	})
	driver.listen(window, "pointercancel", func(event js.Value) {
		if event.Get("pointerType").String() != touchPointerType {
			driver.input.MouseButtons = [8]bool{}
		}
	})
	driver.listen(driver.canvas, "wheel", driver.readWheel)
	driver.listen(driver.canvas, "contextmenu", func(event js.Value) { event.Call("preventDefault") })
	driver.listen(driver.canvas, "click", func(event js.Value) {
		if driver.uiBridge != nil && driver.uiBridge.consumesPointer(event) {
			return
		}

		if driver.lastPointerTouch || emulatedTouchMouse(event) || !driver.pointerLockAllowed {
			return
		}

		if driver.canvas.Get("requestPointerLock").Type() == js.TypeFunction {
			driver.canvas.Call("requestPointerLock")
		}
	})
}

func (driver *Driver) setMouseButton(event js.Value, down bool) {
	button := event.Get("button").Int()
	if button >= 0 && button < len(driver.input.MouseButtons) {
		driver.input.MouseButtons[button] = down
	}
}

func emulatedTouchMouse(event js.Value) bool {
	capabilities := event.Get("sourceCapabilities")

	return capabilities.Type() == js.TypeObject && !capabilities.IsNull() && capabilities.Get("firesTouchEvents").Truthy()
}

func (driver *Driver) setPointerLockMode(allowed bool) {
	driver.pointerLockAllowed = allowed
	if allowed || !driver.input.PointerLocked {
		return
	}

	js.Global().Get("document").Call("exitPointerLock")

	driver.input.PointerLocked = false
	driver.input.MouseDX, driver.input.MouseDY = 0, 0
}

func (driver *Driver) resetInputDeltas() {
	driver.input.MouseDX, driver.input.MouseDY = 0, 0
	clear(driver.input.DragDX[:])
	clear(driver.input.DragDY[:])
	driver.input.WheelX, driver.input.WheelY = 0, 0
	driver.input.TouchDX, driver.input.TouchDY = 0, 0
	driver.input.TouchPanX, driver.input.TouchPanY = 0, 0
	driver.input.Pinch = 0
	driver.input.Touch.Pressed = false
}

func (driver *Driver) listen(target js.Value, event string, callback func(js.Value)) {
	function := js.FuncOf(func(_ js.Value, args []js.Value) any {
		if !driver.closed && len(args) > 0 {
			callback(args[0])
		}

		return nil
	})
	// Explicitly non-passive so wheel/touch handlers can consume canvas gestures.
	target.Call("addEventListener", event, function, jsObject(map[string]any{"passive": false}))
	driver.listeners = append(driver.listeners, inputListener{target: target, event: event, fn: function})
}

func (driver *Driver) removeInputListeners() {
	for _, listener := range driver.listeners {
		listener.target.Call("removeEventListener", listener.event, listener.fn)
		listener.fn.Release()
	}

	driver.listeners = nil
}

func browserKey(code string) uint8 {
	switch code {
	case "Digit1":
		return platform.KeyCode1
	case "Digit2":
		return platform.KeyCode2
	case "Digit3":
		return platform.KeyCode3
	case "KeyB":
		return platform.KeyCodeB
	case "KeyH":
		return platform.KeyCodeH
	case "KeyL":
		return platform.KeyCodeL
	case "KeyR":
		return platform.KeyCodeR
	case "KeyA":
		return platform.KeyCodeA
	case "KeyD":
		return platform.KeyCodeD
	case "KeyO":
		return platform.KeyCodeO
	case "KeyN":
		return platform.KeyCodeN
	case "KeyS":
		return platform.KeyCodeS
	case "KeyT":
		return platform.KeyCodeT
	case "KeyW":
		return platform.KeyCodeW
	case "Space":
		return platform.KeyCodeSpace
	case "ShiftLeft", "ShiftRight":
		return platform.KeyCodeShift
	default:
		return 0
	}
}
