//go:build !js

package debug_test

import (
	"testing"
	"time"

	"github.com/karty-game/karty-webgpu/pkg/debug"
	"github.com/karty-game/karty-webgpu/pkg/engine"
)

func TestPanelSamplingAndLifecycle(t *testing.T) {
	t.Parallel()

	app := engine.NewApp(engine.AppConfig{})

	panel, err := debug.Show(app)
	if err != nil {
		t.Fatal(err)
	}

	if !panel.Active() {
		t.Fatal("shown panel is inactive")
	}

	frame := engine.FrameStats{
		Interval:  100 * time.Millisecond,
		Timings:   engine.FrameTimings{Update: time.Millisecond, Total: 2 * time.Millisecond},
		DrawCalls: 2, LoadedTextures: 3, ResidentTextures: 4, TextureBytes: 64,
	}
	for range 4 {
		panel.Observe(frame)
	}

	report := panel.Stats()
	if report.FPS != 10 || report.Frame.DrawCalls != 2 || report.Frame.Timings.Update != time.Millisecond {
		t.Fatalf("sampled report: %+v", report)
	}

	if report.GoHeapBytes == 0 || report.RefreshDuration <= 0 || report.JSHeapAvailable {
		t.Fatalf("native memory/refresh report: %+v", report)
	}

	panel.Hide()

	if panel.Active() {
		t.Fatal("hidden panel is active")
	}

	panel.Observe(engine.FrameStats{Interval: time.Second, DrawCalls: 99})

	if panel.Stats() != report {
		t.Fatal("hidden panel changed its sampled report")
	}

	panel.Show()
	panel.Observe(engine.FrameStats{Interval: time.Hour})

	for range 3 {
		panel.Observe(frame)
	}

	if panel.Stats().FPS != 10 {
		t.Fatal("resumed sample included time spent hidden")
	}

	if err := app.Close(); err != nil {
		t.Fatal(err)
	}

	if panel.Active() {
		t.Fatal("App close did not close panel")
	}

	panel.Close()
	panel.Show()

	if panel.Active() {
		t.Fatal("closed panel reopened")
	}
}

//nolint:paralleltest // testing.AllocsPerRun must remain isolated on Go 1.27.
func TestPanelNonRefreshFramesDoNotAllocate(t *testing.T) {
	app := engine.NewApp(engine.AppConfig{})

	panel, err := debug.Show(app)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		if err := app.Close(); err != nil {
			t.Error(err)
		}
	})

	frame := engine.FrameStats{Interval: time.Nanosecond}
	panel.Observe(frame)

	allocations := testing.AllocsPerRun(1000, func() {
		panel.Observe(frame)
		_ = panel.Stats()
	})
	if allocations != 0 {
		t.Fatalf("non-refresh panel frames allocate %g objects", allocations)
	}
}
