package diagnostics_test

import (
	"testing"
	"time"

	"github.com/karty-game/karty-webgpu/internal/diagnostics"
)

type observer struct {
	active bool
	frames int
	latest diagnostics.Stats
}

func (value *observer) Active() bool                    { return value.active }
func (value *observer) Observe(stats diagnostics.Stats) { value.frames++; value.latest = stats }
func (*observer) Close()                                {}

func TestCollectorCountsActualWorkAndStopsWhenInactive(t *testing.T) {
	t.Parallel()

	sink := &observer{active: true}
	monitor := diagnostics.Monitor{Observer: sink}
	monitor.Begin(time.Second / 60)
	monitor.Step(diagnostics.UpdatePhase)
	monitor.Draw(6, 10)
	monitor.Dispatch()
	monitor.Loaded(3)
	monitor.Resources(2, 1024)
	monitor.Finish()

	if sink.frames != 1 || sink.latest.DrawCalls != 1 || sink.latest.Instances != 10 || sink.latest.Triangles != 20 ||
		sink.latest.ComputeDispatches != 1 ||
		sink.latest.LoadedTextures != 3 ||
		sink.latest.ResidentTextures != 2 ||
		sink.latest.TextureBytes != 1024 {
		t.Fatalf("incorrect completed-frame counters: %+v", sink)
	}

	if sink.latest.Timings.Total < sink.latest.Timings.Update {
		t.Fatal("phase exceeds total CPU time")
	}

	sink.active = false

	monitor.Begin(time.Second)
	monitor.Draw(3, 1)
	monitor.Finish()

	if sink.frames != 1 {
		t.Fatal("inactive observer received a frame")
	}
}

//nolint:paralleltest // Allocation counters must run serially on Go 1.27.
func TestCollectorDoesNotAllocate(t *testing.T) {
	// Allocation counters must be tested without t.Parallel on Go 1.27.
	for _, active := range []bool{false, true} {
		sink := &observer{active: active}
		monitor := diagnostics.Monitor{Observer: sink}

		allocations := testing.AllocsPerRun(1000, func() {
			monitor.Begin(time.Second / 60)
			monitor.Step(diagnostics.UpdatePhase)
			monitor.Draw(6, 10)
			monitor.DrawSprites(6, 1620)
			monitor.DrawUI(6, 392)
			monitor.Dispatch()
			monitor.Resources(2, 1024)
			monitor.Finish()
		})
		if allocations != 0 {
			t.Fatalf("active=%t: allocated %g objects per frame", active, allocations)
		}
	}
}

func TestLayerCountersExplainModelAndDebugTotals(t *testing.T) {
	t.Parallel()

	sink := &observer{active: true}
	monitor := diagnostics.Monitor{Observer: sink}
	monitor.Begin(time.Second / 60)
	monitor.Draw(36, 2)
	monitor.Draw(36, 2)
	monitor.Draw(6, 1)
	monitor.DrawUI(6, 392)
	monitor.Finish()

	frame := sink.latest
	if frame.Scene != (diagnostics.DrawStats{DrawCalls: 3, Instances: 5, Triangles: 50}) ||
		frame.UI != (diagnostics.DrawStats{DrawCalls: 1, Instances: 392, Triangles: 784}) ||
		frame.Sprites != (diagnostics.DrawStats{}) {
		t.Fatal("scene geometry was mixed with debug quads", frame)
	}

	if frame.DrawCalls != 4 || frame.Instances != 397 || frame.Triangles != 834 {
		t.Fatal("existing total counters changed semantics", frame)
	}

	monitor.Begin(time.Second / 60)
	monitor.DrawSprites(6, 1620)
	monitor.DrawUI(6, 10)
	monitor.Finish()

	frame = sink.latest
	if frame.Scene != (diagnostics.DrawStats{}) || frame.Sprites.Instances != 1620 || frame.Sprites.Triangles != 3240 ||
		frame.UI.Instances != 10 ||
		frame.Instances != 1630 ||
		frame.Triangles != 3260 {
		t.Fatal("sprite counts leaked into scene/UI or retained last frame's values", frame)
	}

	sink.active = false

	monitor.Begin(time.Second)
	monitor.DrawUI(6, 100)
	monitor.DrawSprites(6, 100)
	monitor.Finish()

	if sink.frames != 2 {
		t.Fatal("inactive layer counters delivered a frame")
	}
}
