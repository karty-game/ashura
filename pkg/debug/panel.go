// Package debug provides an optional retained UI performance panel and sampled Go
// runtime statistics. Importing this package is not required by the engine.
package debug

import (
	"runtime/metrics"
	"time"

	"github.com/karty-game/karty-webgpu/pkg/engine"
)

const refreshInterval = time.Second / 4

// Report contains the most recent sampled metrics. Frame timings and interval
// are window averages; draw/resource counts describe the most recent frame.
// Go allocation counters can update in batches when runtime allocation statistics
// flush; a zero sampled window does not prove zero allocations. JavaScript heap
// changes are net live-heap changes, never allocation counts.
type Report struct {
	Frame                  engine.FrameStats
	FPS                    float64
	GoHeapBytes            uint64
	GoHeapAvailable        bool
	GoLiveHeapBytes        uint64
	GoLiveHeapAvailable    bool
	GoBytesPerFrame        float64
	GoObjectsPerFrame      float64
	GoBytesPerSecond       float64
	GoAllocationsAvailable bool
	JSHeapBytes            uint64
	JSHeapAvailable        bool
	JSHeapDeltaPerSecond   float64
	JSHeapDeltaAvailable   bool
	RefreshDuration        time.Duration
}

// Panel owns a retained UI overlay and its metrics sampling. Its methods belong
// to the App loop goroutine. Closing the App also closes its panel.
type Panel struct {
	ui               panelUI
	report           Report
	samples          [4]metrics.Sample
	timings          engine.FrameTimings
	interval         time.Duration
	frames           int64
	lastBytes        uint64
	lastObjects      uint64
	lastJSHeap       uint64
	lastSample       time.Time
	hadJSHeap        bool
	hadGoAllocations bool
	visible          bool
	closed           bool
	fresh            bool
	stopUpdate       func()
}

// Show attaches a panel to app and enables collection. It replaces any previous
// frame observer. F3 toggles the panel; Escape hides it. There are no
// independent timers, animation callbacks or memory samples while hidden.
// Configure App.SetUI before Show, then call Run; the existing application UI
// remains attached beneath the panel.
func Show(app *engine.App) (*Panel, error) {
	if app.Closed() {
		return nil, engine.ErrClosed
	}

	panel := &Panel{}
	panel.samples[0].Name = "/gc/heap/allocs:bytes"
	panel.samples[1].Name = "/gc/heap/allocs:objects"
	panel.samples[2].Name = "/memory/classes/heap/objects:bytes"
	panel.samples[3].Name = "/gc/heap/live:bytes"

	if err := panel.ui.open(app, panel); err != nil {
		return nil, err
	}

	panel.stopUpdate = app.OnUpdateError(func(time.Duration) error { return panel.Err() })
	panel.Show()
	app.SetFrameObserver(panel)

	return panel, nil
}

// Active reports whether frame diagnostics should be collected.
func (panel *Panel) Active() bool {
	return panel != nil && panel.visible && !panel.closed
}

// Show makes the panel visible and resets its sampling baseline.
func (panel *Panel) Show() {
	if panel == nil || panel.closed || panel.visible {
		return
	}

	panel.visible = true
	panel.fresh = true
	panel.frames, panel.interval = 0, 0
	panel.timings = engine.FrameTimings{}
	panel.hadJSHeap = false
	panel.hadGoAllocations = false
	panel.ui.show(true)
}

// Hide suspends metrics collection and keeps a small UI button to reopen it.
func (panel *Panel) Hide() {
	if !panel.Active() {
		return
	}

	panel.visible = false
	panel.ui.show(false)
}

// Close stops collection and removes the overlay, shortcuts and error callback.
func (panel *Panel) Close() {
	if panel == nil || panel.closed {
		return
	}

	panel.closed = true

	panel.visible = false
	if panel.stopUpdate != nil {
		panel.stopUpdate()
		panel.stopUpdate = nil
	}

	panel.ui.close()
}

// Err reports a retained UI update failure. The App also returns it through Run.
// Read and mutate the panel only on the App loop goroutine.
func (panel *Panel) Err() error {
	if panel == nil {
		return nil
	}

	return panel.ui.failure
}

// Stats returns a value snapshot without allocating or reading runtime counters.
func (panel *Panel) Stats() Report {
	if panel == nil {
		return Report{}
	}

	return panel.report
}

// Observe consumes completed frame diagnostics. Non-refresh frames only update
// value counters; runtime memory samples and UI text updates happen at most four times
// per second. The refresh cost is reported separately from engine CPU timing.
func (panel *Panel) Observe(frame engine.FrameStats) {
	if !panel.Active() {
		return
	}

	if panel.fresh {
		// Exclude device startup and the interval spent hidden from both
		// allocation rates and frame averages.
		metrics.Read(panel.samples[:])
		panel.lastBytes, panel.lastObjects, panel.hadGoAllocations = panel.goAllocations()
		panel.lastSample = time.Now()
		panel.fresh = false

		return
	}

	if frame.Interval <= 0 {
		return
	}

	panel.frames++
	panel.interval += frame.Interval
	panel.addTimings(frame.Timings)

	if panel.interval < refreshInterval {
		return
	}

	panel.refresh(frame)
}

func (panel *Panel) refresh(frame engine.FrameStats) {
	started := time.Now()
	seconds := started.Sub(panel.lastSample).Seconds()
	metrics.Read(panel.samples[:])
	bytesAllocated, objectsAllocated, available := panel.goAllocations()
	frame.Interval = panel.interval / time.Duration(panel.frames)
	frame.Timings = panel.averageTimings()
	panel.report.Frame = frame
	panel.report.FPS = float64(time.Second) / float64(frame.Interval)
	panel.report.GoHeapBytes = 0

	panel.report.GoHeapAvailable = panel.samples[2].Value.Kind() == metrics.KindUint64
	if panel.report.GoHeapAvailable {
		panel.report.GoHeapBytes = panel.samples[2].Value.Uint64()
	}

	panel.report.GoLiveHeapBytes = 0

	panel.report.GoLiveHeapAvailable = panel.samples[3].Value.Kind() == metrics.KindUint64
	if panel.report.GoLiveHeapAvailable {
		panel.report.GoLiveHeapBytes = panel.samples[3].Value.Uint64()
	}

	panel.report.GoAllocationsAvailable = available && panel.hadGoAllocations && seconds > 0 &&
		bytesAllocated >= panel.lastBytes && objectsAllocated >= panel.lastObjects

	panel.report.GoBytesPerFrame, panel.report.GoObjectsPerFrame, panel.report.GoBytesPerSecond = 0, 0, 0
	if panel.report.GoAllocationsAvailable {
		panel.report.GoBytesPerFrame = float64(bytesAllocated-panel.lastBytes) / float64(panel.frames)
		panel.report.GoObjectsPerFrame = float64(objectsAllocated-panel.lastObjects) / float64(panel.frames)
		panel.report.GoBytesPerSecond = float64(bytesAllocated-panel.lastBytes) / seconds
	}

	panel.ui.readHeap(&panel.report, panel.lastJSHeap, panel.hadJSHeap, seconds)
	panel.lastJSHeap = panel.report.JSHeapBytes
	panel.hadJSHeap = panel.report.JSHeapAvailable
	panel.ui.render(panel.report)
	panel.lastBytes, panel.lastObjects = bytesAllocated, objectsAllocated
	panel.hadGoAllocations = available
	panel.lastSample = started
	panel.frames, panel.interval = 0, 0
	panel.timings = engine.FrameTimings{}
	panel.report.RefreshDuration = time.Since(started)
}

func (panel *Panel) goAllocations() (uint64, uint64, bool) {
	if panel.samples[0].Value.Kind() != metrics.KindUint64 || panel.samples[1].Value.Kind() != metrics.KindUint64 {
		return 0, 0, false
	}

	return panel.samples[0].Value.Uint64(), panel.samples[1].Value.Uint64(), true
}

func (panel *Panel) addTimings(value engine.FrameTimings) {
	panel.timings.Update += value.Update
	panel.timings.Schedule += value.Schedule
	panel.timings.Scene += value.Scene
	panel.timings.Prepare += value.Prepare
	panel.timings.Surface += value.Surface
	panel.timings.Compute += value.Compute
	panel.timings.Render += value.Render
	panel.timings.Submit += value.Submit
	panel.timings.Total += value.Total
}

func (panel *Panel) averageTimings() engine.FrameTimings {
	count := time.Duration(panel.frames)

	return engine.FrameTimings{
		Update: panel.timings.Update / count, Schedule: panel.timings.Schedule / count,
		Scene: panel.timings.Scene / count, Prepare: panel.timings.Prepare / count,
		Surface: panel.timings.Surface / count, Compute: panel.timings.Compute / count,
		Render: panel.timings.Render / count, Submit: panel.timings.Submit / count,
		Total: panel.timings.Total / count,
	}
}
