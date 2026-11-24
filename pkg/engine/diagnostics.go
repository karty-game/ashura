package engine

import "github.com/karty-game/ashura/internal/diagnostics"

// DrawStats counts submitted draws, instances and triangles in one rendering layer.
// Scene instances are mesh submissions; sprite and UI instances are quads.
type DrawStats = diagnostics.DrawStats

// FrameStats is one completed CPU frame's timings, actual draws and texture counters.
// Flat draw counters total Scene, Sprites and UI, including the debug panel.
// Interval measures loop cadence; it does not prove physical presentation or GPU time.
type FrameStats = diagnostics.Stats

// FrameTimings separates CPU wall time by execution phase. Render includes
// material preparation and uploads; native submission/presentation may wait.
// Total excludes diagnostic observer work, which can still affect frame cadence.
type FrameTimings = diagnostics.Timings

// FrameObserver collects optional diagnostics. Active is checked once per frame;
// inactive observers cause no timing reads or per-frame metric allocations.
// Observe runs on the loop goroutine after a successful frame; keep it lightweight.
// Close releases the observer when replaced or when the App closes.
type FrameObserver = diagnostics.Observer

// FrameObserverFunc observes every frame with a callback, including on native.
// Use a custom FrameObserver when collection needs to be paused or own resources.
type FrameObserverFunc func(FrameStats)

// Active enables collection while the callback is non-nil.
func (observe FrameObserverFunc) Active() bool { return observe != nil }

// Observe delivers one completed frame to the callback.
func (observe FrameObserverFunc) Observe(stats FrameStats) { observe(stats) }

// Close has no resources to release for a function observer.
func (FrameObserverFunc) Close() {}

// SetFrameObserver replaces the App's diagnostic observer, closing the previous
// one. Pass nil to disable collection. Call on the App loop goroutine, or before
// Run. The optional debug package uses this hook; no panel is linked by default.
func (app *App) SetFrameObserver(observer FrameObserver) {
	if app == nil || app.closed {
		if observer != nil {
			observer.Close()
		}

		return
	}

	previous := app.metrics.Observer
	app.metrics.Observer = observer
	app.metrics.Discard()

	if previous != nil {
		previous.Close()
	}
}

func (app *App) closeFrameObserver() {
	previous := app.metrics.Observer
	app.metrics.Observer = nil
	app.metrics.Discard()

	if previous != nil {
		previous.Close()
	}
}
