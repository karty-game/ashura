package engine

import (
	_ "embed"
	"errors"
	"testing"
	"time"

	"github.com/karty-game/karty-webgpu/internal/platform"
	"github.com/karty-game/karty-webgpu/pkg/compute"
	"github.com/karty-game/karty-webgpu/pkg/shader"
)

//go:embed wgsl/compute-fixture.wgsl
var scheduleWGSL string

func TestComputeScheduleAndBorrowedRecorder(t *testing.T) {
	t.Parallel()

	app := NewApp(AppConfig{})
	defer func() { _ = app.Close() }()

	const uniformSize = 16

	params, err := shader.NewUniformBytes(app, make([]byte, uniformSize))
	if err != nil {
		t.Fatal(err)
	}

	kernel, err := compute.NewPipeline(app, compute.PipelineDesc{
		Program: shader.WGSL("wgsl/compute-fixture.wgsl", scheduleWGSL), EntryPoint: "main",
		Layout: shader.Layout{shader.UniformLayout(0, uniformSize)},
	})
	if err != nil {
		t.Fatal(err)
	}

	group, err := kernel.BindGroup(shader.BindBuffer(0, params))
	if err != nil {
		t.Fatal(err)
	}

	var (
		stage       int
		initialized bool
		saved       Frame
		borrowed    *Frame
	)

	app.OnUpdate(func(time.Duration) { stage = 1 })
	app.OnCompute(func(frame *Frame) error {
		if stage != 1 {
			t.Fatal("compute ran before the CPU update callback")
		}

		stage = 2

		if initialized {
			return nil
		}

		if err := params.Write(0, []byte{7, 0, 0, 0}); err != nil {
			return err
		}

		borrowed = frame

		saved = *frame
		if err := saved.Dispatch(kernel, group, 1, 1, 1); err != nil {
			return err
		}

		if err := app.UnloadComputeBindings(group); !errors.Is(err, errComputeInUse) {
			t.Fatalf("unloading queued bindings returned %v, want in-use error", err)
		}

		initialized = true

		return nil
	})

	scene := NewWorld()

	frame, err := app.updateFrame(scene, time.Second, platform.InputState{})
	if err != nil {
		t.Fatal(err)
	}

	if stage != 2 || len(frame.Compute) != 1 || frame.Compute[0].Pipeline != kernel || frame.Compute[0].Group != group {
		t.Fatal("copied recorder did not append to the App's scheduled commands")
	}

	if len(frame.Uploads) != 1 || frame.Uploads[0].Buffer != params || frame.Uploads[0].Data[0] != 7 {
		t.Fatal("write from OnCompute was missing from the same frame's uploads")
	}

	if err := saved.Dispatch(kernel, group, 1, 1, 1); !errors.Is(err, errComputePhase) {
		t.Fatalf("saved value dispatched outside the callback: %v", err)
	}

	if err := borrowed.Dispatch(kernel, group, 1, 1, 1); !errors.Is(err, errComputePhase) {
		t.Fatalf("borrowed pointer dispatched outside the callback: %v", err)
	}

	frame, err = app.updateFrame(scene, time.Second, platform.InputState{})
	if err != nil {
		t.Fatal(err)
	}

	if len(frame.Uploads) != 0 || len(frame.Compute) != 0 {
		t.Fatal("unchanged next frame repeated uploads or compute commands")
	}
}
