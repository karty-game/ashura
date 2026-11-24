//go:build js && wasm

package browser

import (
	"syscall/js"

	"github.com/karty-game/ashura/internal/platform"
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
		driver.touch.BlockPointer(&driver.input)
		driver.readTouch(event)
	})
}

func (driver *Driver) readTouch(event js.Value) {
	if event.Get("cancelable").Bool() {
		event.Call("preventDefault")
	}

	driver.lastPointerTouch = true
	touches := event.Get("targetTouches")
	count := touches.Length()

	if driver.uiBridge != nil && driver.uiBridge.capturesPointer() {
		driver.touch.Reset()
		driver.touch.BlockPointer(&driver.input)

		if count == 0 {
			driver.touch.Reset()
		}

		return
	}

	if count > maximumTouches {
		driver.touch.Reset()
		driver.touch.BlockPointer(&driver.input)

		return
	}

	if count == 0 {
		driver.touch.Update(&driver.input, nil)

		return
	}

	var points [maximumTouches]platform.TouchPoint

	bounds := driver.canvas.Call("getBoundingClientRect")
	left, top := bounds.Get("left").Float(), bounds.Get("top").Float()

	for index := range count {
		touch := touches.Index(index)
		points[index] = platform.TouchPoint{
			ID: int64(touch.Get("identifier").Int()),
			X:  float32(touch.Get("clientX").Float() - left),
			Y:  float32(touch.Get("clientY").Float() - top),
		}
	}

	if count == 1 && event.Get("type").String() == "touchstart" &&
		driver.uiBridge != nil && driver.uiBridge.consumesPointerAt(points[0].X, points[0].Y) {
		driver.touch.BlockPointer(&driver.input)
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
