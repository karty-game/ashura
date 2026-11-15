// Package diagnostics holds optional CPU-side frame instrumentation.
package diagnostics

import "time"

// Timings are wall durations of CPU phases, not GPU execution durations.
// Render includes material preparation/uploads; Submit can include native waits.
type Timings struct {
	Update, Schedule, Scene, Prepare, Surface, Compute, Render, Submit, Total time.Duration
}

// DrawStats counts submitted work in one rendering layer, not logical entities.
type DrawStats struct {
	DrawCalls, Instances, Triangles uint64
}

// Stats describes one completed frame. Flat draw counters total Scene, Sprites
// and UI, including the debug panel. GPU texture bytes estimate RGBA8 payloads,
// excluding render targets and implementation-specific padding/driver allocations.
type Stats struct {
	Interval                                           time.Duration
	Timings                                            Timings
	DrawCalls, Instances, Triangles, ComputeDispatches uint64
	Scene, Sprites, UI                                 DrawStats
	LoadedTextures, ResidentTextures                   int
	TextureBytes                                       uint64
}

// Observer receives frames only while Active. The App closes its observer.
type Observer interface {
	Active() bool
	Observe(stats Stats)
	Close()
}

// Phase identifies a boundary in the CPU frame; all phases are sequential.
type Phase uint8

const (
	UpdatePhase Phase = iota
	SchedulePhase
	ScenePhase
	PreparePhase
	SurfacePhase
	ComputePhase
	RenderPhase
	SubmitPhase
)

// Monitor reuses one snapshot. Disabled collection performs no clock reads,
// allocations, resource scans, GPU queries or observer calls.
type Monitor struct {
	Observer          Observer
	current           Stats
	started, boundary time.Time
	active            bool
	frameObserver     Observer
}

// Begin selects the observer once, keeping a consistent active state per frame.
func (monitor *Monitor) Begin(interval time.Duration) {
	if monitor == nil {
		return
	}

	monitor.active = monitor.Observer != nil && monitor.Observer.Active()
	if !monitor.active {
		return
	}

	monitor.frameObserver = monitor.Observer
	monitor.current = Stats{Interval: interval}
	monitor.started = time.Now()
	monitor.boundary = monitor.started
}

// Enabled reports whether collection is active for the current frame.
func (monitor *Monitor) Enabled() bool { return monitor != nil && monitor.active }

// Step records time since the previous boundary, without heap storage.
func (monitor *Monitor) Step(phase Phase) {
	if !monitor.Enabled() {
		return
	}

	now := time.Now()
	elapsed := now.Sub(monitor.boundary)
	monitor.boundary = now

	switch phase {
	case UpdatePhase:
		monitor.current.Timings.Update += elapsed
	case SchedulePhase:
		monitor.current.Timings.Schedule += elapsed
	case ScenePhase:
		monitor.current.Timings.Scene += elapsed
	case PreparePhase:
		monitor.current.Timings.Prepare += elapsed
	case SurfacePhase:
		monitor.current.Timings.Surface += elapsed
	case ComputePhase:
		monitor.current.Timings.Compute += elapsed
	case RenderPhase:
		monitor.current.Timings.Render += elapsed
	case SubmitPhase:
		monitor.current.Timings.Submit += elapsed
	}
}

// Draw records an actual scene draw; instancing does not multiply draw calls.
func (monitor *Monitor) Draw(indices, instances uint64) {
	if !monitor.Enabled() {
		return
	}

	monitor.recordDraw(&monitor.current.Scene, indices, instances)
}

// DrawSprites records one submitted sprite batch, also included in totals.
func (monitor *Monitor) DrawSprites(vertices, instances uint64) {
	if !monitor.Enabled() {
		return
	}

	monitor.recordDraw(&monitor.current.Sprites, vertices, instances)
}

// DrawUI records one submitted UI batch, including diagnostic UI, in totals.
func (monitor *Monitor) DrawUI(vertices, instances uint64) {
	if !monitor.Enabled() {
		return
	}

	monitor.recordDraw(&monitor.current.UI, vertices, instances)
}

// Dispatch records one actual compute dispatch.
func (monitor *Monitor) Dispatch() {
	if monitor.Enabled() {
		monitor.current.ComputeDispatches++
	}
}

// Loaded reports App-owned live texture handles, including unused handles.
func (monitor *Monitor) Loaded(count int) {
	if monitor.Enabled() {
		monitor.current.LoadedTextures = count
	}
}

// Resources reports cached sampled/storage textures, including fallbacks.
func (monitor *Monitor) Resources(resident int, bytes uint64) {
	if !monitor.Enabled() {
		return
	}

	monitor.current.ResidentTextures, monitor.current.TextureBytes = resident, bytes
}

// Finish excludes observer sampling, formatting and UI updates from Total.
// Their cost still affects the next frame interval and observed cadence.
func (monitor *Monitor) Finish() {
	if !monitor.Enabled() {
		return
	}

	monitor.current.Timings.Total = time.Since(monitor.started)
	monitor.active = false
	observer := monitor.frameObserver
	monitor.frameObserver = nil
	observer.Observe(monitor.current)
}

// Discard drops an interrupted frame and any observer reference it retained.
func (monitor *Monitor) Discard() {
	monitor.active = false
	monitor.frameObserver = nil
}

func (monitor *Monitor) recordDraw(layer *DrawStats, vertices, instances uint64) {
	const triangleIndices = 3

	triangles := vertices / triangleIndices * instances
	layer.DrawCalls++
	layer.Instances += instances
	layer.Triangles += triangles
	monitor.current.DrawCalls++
	monitor.current.Instances += instances
	monitor.current.Triangles += triangles
}
