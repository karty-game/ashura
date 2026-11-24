//go:build js && wasm

package main

import (
	_ "embed"
	"fmt"
	"syscall/js"
	"time"

	"github.com/karty-game/ashura/pkg/engine"
)

//go:embed touch.html
var touchMarkup string

type domListener struct {
	target js.Value
	name   string
	fn     js.Func
}

type touchOverlay struct {
	listeners []domListener
}

func attachTouchControls(canvasID string, input *courtyardInput) (present func(trialStatus, bool), cleanup func()) {
	document := js.Global().Get("document")

	canvas := document.Call("getElementById", canvasID)
	if canvas.IsNull() {
		return func(trialStatus, bool) {}, func() {}
	}

	root := document.Call("createElement", "div")
	root.Set("id", "courtyard-touch")
	root.Set("innerHTML", touchMarkup)
	document.Get("body").Call("appendChild", root)

	stick := root.Call("querySelector", "#courtyard-stick")
	knob := root.Call("querySelector", "#courtyard-knob").Get("style")
	use := root.Call("querySelector", "#courtyard-use")
	restart := root.Call("querySelector", "#courtyard-restart")

	var overlay touchOverlay

	resetStick := overlay.bindStick(stick, func(offset engine.Vector2) {
		input.move = stickMovement(offset)
		knob.Set("transform", fmt.Sprintf("translate(%gpx,%gpx)", input.move.X*stickRadius, -input.move.Y*stickRadius))
	})
	overlay.listen(use, "click", func(js.Value) { input.use = true })
	overlay.listen(restart, "click", func(js.Value) {
		input.restart = true

		canvas.Call("focus")
	})

	reset := func() {
		resetStick()

		*input = courtyardInput{}
	}

	overlay.listen(js.Global(), "blur", func(js.Value) { reset() })
	overlay.listen(document, "visibilitychange", func(js.Value) {
		if document.Get("hidden").Bool() {
			reset()
		}
	})

	return trialPresenter(root, use, restart), func() {
		overlay.close()
		root.Call("remove")
	}
}

func trialPresenter(root, use, restart js.Value) func(trialStatus, bool) {
	hud := root.Call("querySelector", "#courtyard-hud")
	relics := root.Call("querySelector", "#courtyard-relics")
	clock := root.Call("querySelector", "#courtyard-clock")
	timer := root.Call("querySelector", "#courtyard-gate-time")
	instruction := root.Call("querySelector", "#courtyard-objective")
	reticle := root.Call("querySelector", "#courtyard-reticle")

	var (
		previous  trialStatus
		debugOpen bool
	)

	return func(next trialStatus, showDebug bool) {
		if showDebug != debugOpen {
			root.Get("classList").Call("toggle", "debug-open", showDebug)

			debugOpen = showDebug
		}

		next.elapsed, next.best = next.elapsed.Truncate(clockTick), next.best.Truncate(clockTick)
		next.gateTime = roundUpTick(next.gateTime)

		if next == previous {
			return
		}

		if next.route != previous.route || next.collected != previous.collected || previous.instruction == "" {
			relics.Set("textContent", fmt.Sprintf("Route %c · Relics %d/%d", 'A'+next.route, next.collected, relicCount))
		}

		if next.elapsed != previous.elapsed || next.best != previous.best || previous.instruction == "" {
			clock.Set("textContent", clockText(next))
		}

		if next.instruction != previous.instruction {
			instruction.Set("textContent", next.instruction)
		}

		if next.gateTime != previous.gateTime {
			timer.Set("hidden", next.gateTime == 0)
			timer.Set("textContent", fmt.Sprintf("Gate %.1fs", next.gateTime.Seconds()))
		}

		if next.switchReady != previous.switchReady {
			use.Set("disabled", !next.switchReady)
			reticle.Get("classList").Call("toggle", "ready", next.switchReady)
		}

		if next.finished != previous.finished {
			hud.Get("classList").Call("toggle", "complete", next.finished)

			label := "Restart"
			if next.finished {
				label = "Next route"
			}

			restart.Set("textContent", label)
		}

		previous = next
	}
}

func clockText(status trialStatus) string {
	if status.best == 0 {
		return fmt.Sprintf("Time %.1fs", status.elapsed.Seconds())
	}

	return fmt.Sprintf("Time %.1fs · Best %.1fs", status.elapsed.Seconds(), status.best.Seconds())
}

func roundUpTick(value time.Duration) time.Duration {
	return (value + clockTick - 1).Truncate(clockTick)
}

func (overlay *touchOverlay) listen(target js.Value, name string, fn func(js.Value)) {
	callback := js.FuncOf(func(_ js.Value, args []js.Value) any {
		fn(args[0])

		return nil
	})
	target.Call("addEventListener", name, callback)
	overlay.listeners = append(overlay.listeners, domListener{target: target, name: name, fn: callback})
}

func (overlay *touchOverlay) close() {
	for _, listener := range overlay.listeners {
		listener.target.Call("removeEventListener", listener.name, listener.fn)
		listener.fn.Release()
	}
}

func (overlay *touchOverlay) bindStick(target js.Value, move func(engine.Vector2)) func() {
	var (
		origin   engine.Vector2
		pointer  int
		dragging bool
	)

	clearDrag := func() {
		if !dragging {
			return
		}

		dragging = false

		move(engine.Vector2{})
	}

	overlay.listen(target, "pointerdown", func(event js.Value) {
		if dragging || event.Get("pointerType").String() != "touch" {
			return
		}

		dragging = true
		pointer = event.Get("pointerId").Int()
		origin = eventPosition(event)

		target.Call("setPointerCapture", pointer)
	})
	overlay.listen(target, "pointermove", func(event js.Value) {
		if !dragging || event.Get("pointerId").Int() != pointer {
			return
		}

		point := eventPosition(event)
		move(engine.Vector2{X: point.X - origin.X, Y: point.Y - origin.Y})
	})

	for _, name := range [...]string{"pointerup", "pointercancel", "lostpointercapture"} {
		overlay.listen(target, name, func(event js.Value) {
			if dragging && event.Get("pointerId").Int() == pointer {
				clearDrag()
			}
		})
	}

	return clearDrag
}

func eventPosition(event js.Value) engine.Vector2 {
	return engine.Vector2{X: float32(event.Get("clientX").Float()), Y: float32(event.Get("clientY").Float())}
}

func showCourtyardError(err error) {
	document := js.Global().Get("document")

	message := document.Call("getElementById", "error")
	if !message.IsNull() {
		message.Set("textContent", "Could not start Collision Courtyard.\n"+err.Error())
	}
}
