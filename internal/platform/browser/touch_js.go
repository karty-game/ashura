//go:build js && wasm

package browser

import (
	"syscall/js"

	"github.com/karty-game/karty-webgpu/internal/platform"
)

const (
	// Wheel events specify pixels, lines or pages. Normalize to approximately one
	// mouse-wheel notch: 100 CSS pixels or three text lines. Pages use CSS height.
	wheelPixelsPerStep = 100
	wheelLinesPerStep  = 3
	wheelLineMode      = 1
	wheelPageMode      = 2
	maximumTouches     = 2
)

func (driver *Driver) installTouch() {
	driver.canvas.Get("style").Set("touchAction", cullModeNone)
	driver.listen(driver.canvas, "touchstart", func(event js.Value) {
		driver.canvas.Call("focus")

		if driver.input.PointerLocked {
			js.Global().Get("document").Call("exitPointerLock")

			driver.input.PointerLocked = false
		}

		driver.readTouch(event)
	})
	driver.listen(driver.canvas, "touchmove", driver.readTouch)
	driver.listen(driver.canvas, "touchend", driver.readTouch)
	driver.listen(driver.canvas, "touchcancel", func(event js.Value) {
		driver.touch.Reset()
		driver.readTouch(event)
	})
}

func (driver *Driver) readTouch(event js.Value) {
	if driver.uiBridge != nil && driver.uiBridge.capturesPointer() {
		driver.touch.Reset()
		event.Call("preventDefault")

		return
	}

	event.Call("preventDefault")

	driver.lastPointerTouch = true
	touches := event.Get("targetTouches")

	count := touches.Length()
	if count > maximumTouches {
		driver.touch.Reset()

		return
	}

	var points [maximumTouches]platform.TouchPoint

	for index := range count {
		touch := touches.Index(index)
		points[index] = platform.TouchPoint{
			ID: int64(touch.Get("identifier").Int()),
			X:  float32(touch.Get("clientX").Float()),
			Y:  float32(touch.Get("clientY").Float()),
		}
	}

	driver.touch.Update(&driver.input, points[:count])
}

func (driver *Driver) readWheel(event js.Value) {
	event.Call("preventDefault")

	scale := float32(1.0 / wheelPixelsPerStep)

	switch event.Get("deltaMode").Int() {
	case wheelLineMode:
		scale = 1.0 / wheelLinesPerStep
	case wheelPageMode:
		scale = driver.input.LogicalHeight / wheelPixelsPerStep
	}

	driver.input.WheelX += float32(event.Get("deltaX").Float()) * scale
	driver.input.WheelY += float32(event.Get("deltaY").Float()) * scale
}
