package debug

import (
	"runtime/metrics"
	"testing"
	"time"

	"github.com/karty-game/karty-webgpu/pkg/engine"
)

func TestUnsupportedGoCountersRenderUnavailable(t *testing.T) {
	t.Parallel()

	widgets, _ := newRenderingRows(t)

	panel := &Panel{ui: *widgets, visible: true, fresh: true}
	for index := range panel.samples {
		panel.samples[index].Name = "/unsupported/debug-test:bytes"
	}

	refreshMemoryPanel(panel)

	if report := panel.Stats(); report.GoHeapAvailable || report.GoLiveHeapAvailable || report.GoAllocationsAvailable {
		t.Fatal("unsupported runtime counters presented as valid zero measurements", report)
	}

	for _, row := range []int{rowGoHeap, rowGoLiveHeap, rowGoBytesFrame, rowGoObjectsFrame, rowGoBytesSecond} {
		if panel.ui.text[row] != "unavailable" {
			t.Fatal("unsupported runtime counter rendered a number", row, panel.ui.text[row])
		}
	}
}

func TestGoCounterAvailabilityKeepsHeapAndAllocationBaselinesSeparate(t *testing.T) {
	t.Parallel()

	if len(metrics.All()) == 0 {
		t.Skip("runtime exposes no Go counter metrics")
	}

	widgets, _ := newRenderingRows(t)
	panel := &Panel{ui: *widgets, visible: true, fresh: true}
	panel.samples = [4]metrics.Sample{
		{Name: "/unsupported/debug-test:bytes"},
		{Name: "/gc/heap/allocs:objects"},
		{Name: "/memory/classes/heap/objects:bytes"},
		{Name: "/gc/heap/live:bytes"},
	}
	refreshMemoryPanel(panel)

	if report := panel.Stats(); !report.GoHeapAvailable || !report.GoLiveHeapAvailable || report.GoAllocationsAvailable {
		t.Fatal("missing allocation counter hid supported heap counters", report)
	}

	panel.samples[0].Name = "/gc/heap/allocs:bytes"
	refreshMemoryPanel(panel)

	if panel.Stats().GoAllocationsAvailable {
		t.Fatal("newly available cumulative counters lack a matching baseline")
	}

	refreshMemoryPanel(panel)

	if !panel.Stats().GoAllocationsAvailable {
		t.Fatal("supported counters stayed unavailable after establishing a baseline")
	}
}

func refreshMemoryPanel(panel *Panel) {
	for range 4 {
		panel.Observe(engine.FrameStats{Interval: 100 * time.Millisecond})
	}
}
