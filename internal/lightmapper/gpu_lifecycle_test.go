//go:build !js

package lightmapper

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
)

type gpuLifecycleExecutor struct {
	entered chan struct{}
	wait    bool
	calls   atomic.Uint32
	closes  atomic.Uint32
}

func (*gpuLifecycleExecutor) Device() GPUDevice {
	return GPUDevice{MaxBufferSize: 256 << 20, MaxStorageBufferBindingSize: 128 << 20, MaxStorageBuffersPerShaderStage: 4}
}
func (executor *gpuLifecycleExecutor) Run(ctx context.Context, _ GPUScene, _ GPUTile) ([]byte, error) {
	executor.calls.Add(1)

	if executor.entered != nil {
		close(executor.entered)
	}

	if executor.wait {
		<-ctx.Done()

		return nil, ctx.Err()
	}

	return []byte{0}, nil
}
func (*gpuLifecycleExecutor) SceneUploads() uint64           { return 0 }
func (*gpuLifecycleExecutor) TraceRayCounts() GPUTraceCounts { return GPUTraceCounts{} }
func (executor *gpuLifecycleExecutor) Close() error {
	executor.closes.Add(1)

	return nil
}

func gpuLifecycleFixture(t *testing.T, executor *gpuLifecycleExecutor) (*GPUWorker, GPUScene, GPUTile) {
	t.Helper()
	scene := gpuFixtureScene(t, Scene{Environment: Environment{Sky: Vec3{.2, .2, .2}}})

	packed, err := PackGPUScene(scene)
	if err != nil {
		t.Fatal(err)
	}

	tile, err := PackGPUTile(scene, []Receiver{gpuFixtureReceiver(7, 0)}, Options{Samples: 4, Outputs: OutputRGB | OutputRNM})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	worker := &GPUWorker{
		ctx:      ctx,
		cancel:   cancel,
		executor: executor,
		device:   executor.Device(),
		options:  GPUWorkerOptions{MemoryBytes: 128 << 20},
	}

	t.Cleanup(func() { _ = worker.Close() })

	return worker, packed, tile
}

func TestNativeWorkerAdmissionPrecedesEveryExecutorCall(t *testing.T) {
	t.Parallel()

	executor := &gpuLifecycleExecutor{}
	worker, scene, tile := gpuLifecycleFixture(t, executor)

	tile.Options.Bounces = 33
	if _, err := worker.Run(t.Context(), scene, tile); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}

	if executor.calls.Load() != 0 || executor.closes.Load() != 0 {
		t.Fatal("invalid batch reached native worker")
	}
}

func TestNativeWorkerRejectsConcurrentJobsAndPoisonsCancellation(t *testing.T) {
	t.Parallel()

	executor := &gpuLifecycleExecutor{entered: make(chan struct{}), wait: true}
	worker, scene, tile := gpuLifecycleFixture(t, executor)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	done := make(chan error, 1)

	go func() { _, err := worker.Run(ctx, scene, tile); done <- err }()

	select {
	case <-executor.entered:
	case err := <-done:
		t.Fatal("batch never entered native executor", err)
	}

	if _, err := worker.Run(t.Context(), scene, tile); !errors.Is(err, errGPUWorkerBusy) {
		t.Fatal(err)
	}

	cancel()

	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}

	if _, err := worker.Run(t.Context(), scene, tile); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled worker generation was reused", err)
	}

	if executor.calls.Load() != 1 || executor.closes.Load() != 1 {
		t.Fatal("native generation lifetime", executor.calls.Load(), executor.closes.Load())
	}
}

func TestNativeWorkerRejectsIncompleteResultsAndClosesOnce(t *testing.T) {
	t.Parallel()

	executor := &gpuLifecycleExecutor{}

	worker, scene, tile := gpuLifecycleFixture(t, executor)
	if _, err := worker.Run(t.Context(), scene, tile); !errors.Is(err, ErrInvalid) {
		t.Fatal("incomplete native result accepted", err)
	}

	_ = worker.Close()
	if _, err := worker.Run(t.Context(), scene, tile); !errors.Is(err, ErrInvalid) {
		t.Fatal("failed generation reused", err)
	}

	if executor.calls.Load() != 1 || executor.closes.Load() != 1 {
		t.Fatal("native generation lifetime", executor.calls.Load(), executor.closes.Load())
	}
}
