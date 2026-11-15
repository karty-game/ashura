//go:build js && wasm

package debug

import (
	"math"
	"syscall/js"
)

// Only optional browser heap telemetry is platform-specific. The panel is drawn
// entirely by the retained UI library, with no DOM nodes or event listeners.
func (*panelUI) readHeap(report *Report, previous uint64, hadPrevious bool, seconds float64) {
	memory := js.Global().Get("performance").Get("memory")
	report.JSHeapAvailable = false
	report.JSHeapDeltaAvailable = false

	if memory.IsUndefined() || memory.IsNull() {
		return
	}

	value := memory.Get("usedJSHeapSize")
	if value.Type() != js.TypeNumber {
		return
	}

	bytes := value.Float()
	if math.IsNaN(bytes) || math.IsInf(bytes, 0) || bytes < 0 {
		return
	}

	report.JSHeapAvailable = true

	report.JSHeapBytes = uint64(bytes)
	if hadPrevious && seconds > 0 {
		report.JSHeapDeltaAvailable = true
		report.JSHeapDeltaPerSecond = (bytes - float64(previous)) / seconds
	}
}
