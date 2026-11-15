package engine

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/karty-game/karty-webgpu/internal/platform"
)

var errUpdateFixture = errors.New("update fixture failed")

func TestUpdateCallbacksCancelAndRegisterDuringDispatch(t *testing.T) {
	t.Parallel()

	app := NewApp(AppConfig{})
	defer func() { _ = app.Close() }()

	var (
		calls                     []string
		cancelFirst, cancelSecond func()
	)

	cancelFirst = app.OnUpdate(func(time.Duration) {
		calls = append(calls, "first")

		cancelFirst()
		cancelSecond()
		app.OnUpdate(func(time.Duration) { calls = append(calls, "new") })
	})
	cancelSecond = app.OnUpdateError(func(time.Duration) error {
		t.Fatal("canceled callback ran")

		return nil
	})
	app.OnUpdate(func(time.Duration) { calls = append(calls, "third") })

	if err := app.runUpdates(time.Second); err != nil {
		t.Fatal(err)
	}

	if !slices.Equal(calls, []string{"first", "third"}) {
		t.Fatalf("first update calls = %v", calls)
	}

	if len(app.updates.entries) != 2 {
		t.Fatalf("retained callbacks = %d, want only two live entries", len(app.updates.entries))
	}

	cancelFirst()
	cancelSecond()

	calls = nil

	if err := app.runUpdates(time.Second); err != nil {
		t.Fatal(err)
	}

	if !slices.Equal(calls, []string{"third", "new"}) {
		t.Fatalf("second update calls = %v", calls)
	}
}

func TestFallibleUpdateStopsFrameAndPreservesCause(t *testing.T) {
	t.Parallel()

	app := NewApp(AppConfig{})
	defer func() { _ = app.Close() }()

	var calls []string

	app.OnUpdate(func(time.Duration) { calls = append(calls, "before") })
	cancel := app.OnUpdateError(func(time.Duration) error { return errUpdateFixture })
	app.OnUpdate(func(time.Duration) { calls = append(calls, "after") })
	app.OnCompute(func(*Frame) error {
		calls = append(calls, "compute")

		return nil
	})

	world := NewWorld()
	if _, err := app.updateFrame(world, time.Second, platform.InputState{}); !errors.Is(err, errUpdateFixture) {
		t.Fatalf("frame error = %v, want original update cause", err)
	}

	if !slices.Equal(calls, []string{"before"}) {
		t.Fatalf("failed frame calls = %v", calls)
	}

	cancel()

	calls = nil

	if _, err := app.updateFrame(world, time.Second, platform.InputState{}); err != nil {
		t.Fatal(err)
	}

	if !slices.Equal(calls, []string{"before", "after", "compute"}) {
		t.Fatalf("recovered frame calls = %v", calls)
	}
}

func TestComputeCallbacksCancelAndRegisterDuringDispatch(t *testing.T) {
	t.Parallel()

	app := NewApp(AppConfig{})
	defer func() { _ = app.Close() }()

	var (
		calls                     []string
		cancelFirst, cancelSecond func()
	)

	cancelFirst = app.OnCompute(func(*Frame) error {
		calls = append(calls, "first")

		cancelFirst()
		cancelSecond()
		app.OnCompute(func(*Frame) error {
			calls = append(calls, "new")

			return nil
		})

		return nil
	})
	cancelSecond = app.OnCompute(func(*Frame) error {
		t.Fatal("canceled compute callback ran")

		return nil
	})

	if err := app.runCompute(); err != nil {
		t.Fatal(err)
	}

	if !slices.Equal(calls, []string{"first"}) || len(app.computeUpdates.entries) != 1 {
		t.Fatalf("first compute calls = %v; retained callbacks = %d", calls, len(app.computeUpdates.entries))
	}

	calls = nil

	if err := app.runCompute(); err != nil {
		t.Fatal(err)
	}

	if !slices.Equal(calls, []string{"new"}) {
		t.Fatalf("second compute calls = %v", calls)
	}
}

func TestCallbackCancellationAndCloseReleaseClosures(t *testing.T) {
	t.Parallel()

	app := NewApp(AppConfig{})
	cancel := app.OnUpdate(func(time.Duration) {})
	entry := app.updates.entries[0]

	cancel()
	cancel()

	if entry.callback != nil || entry.active || len(app.updates.entries) != 0 {
		t.Fatal("cancellation retained a callback closure or historical registration")
	}

	app.OnUpdateError(func(time.Duration) error { return app.Close() })
	app.OnUpdate(func(time.Duration) { t.Fatal("update after Close ran") })
	app.OnCompute(func(*Frame) error {
		t.Fatal("compute after Close ran")

		return nil
	})

	if err := app.runUpdates(time.Second); !errors.Is(err, ErrClosed) {
		t.Fatalf("closed update error = %v", err)
	}

	if len(app.updates.entries) != 0 || len(app.computeUpdates.entries) != 0 {
		t.Fatal("Close retained update or compute callbacks")
	}

	cancel()
	app.OnUpdate(func(time.Duration) { t.Fatal("closed App accepted update") })()
	app.OnUpdateError(nil)()
	app.OnCompute(nil)()

	var absent *App

	absent.OnUpdate(nil)()
	absent.OnUpdateError(func(time.Duration) error { return nil })()
	absent.OnCompute(func(*Frame) error { return nil })()
}

//nolint:paralleltest // AllocsPerRun must be isolated on Go 1.27.
func TestLiveUpdateAndComputeCallbacksDoNotAllocate(t *testing.T) {
	app := NewApp(AppConfig{})
	defer func() { _ = app.Close() }()

	app.OnUpdate(func(time.Duration) {})
	app.OnUpdateError(func(time.Duration) error { return nil })
	app.OnCompute(func(*Frame) error { return nil })

	var updateErr, computeErr error

	allocations := testing.AllocsPerRun(100, func() {
		updateErr = app.runUpdates(time.Second)
		computeErr = app.runCompute()
	})
	if allocations != 0 || updateErr != nil || computeErr != nil {
		t.Fatalf("callback allocations = %v, errors = %v, %v", allocations, updateErr, computeErr)
	}
}
