//go:build !js

package debug

func (*panelUI) readHeap(report *Report, _ uint64, _ bool, _ float64) {
	report.JSHeapAvailable = false
	report.JSHeapDeltaAvailable = false
}
