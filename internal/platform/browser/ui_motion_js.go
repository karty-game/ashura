//go:build js && wasm

package browser

import "syscall/js"

func (driver *browserUIBridge) installUIMotion() {
	window := js.Global().Get("window")
	if window.Get("matchMedia").Type() != js.TypeFunction {
		return
	}

	query := window.Call("matchMedia", "(prefers-reduced-motion: reduce)")
	driver.ui.SetReducedMotion(query.Get("matches").Bool())
	driver.listen(query, "change", func(event js.Value) { driver.ui.SetReducedMotion(event.Get("matches").Bool()) })
}
