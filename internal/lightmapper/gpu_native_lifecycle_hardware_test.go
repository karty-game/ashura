//go:build !js

package lightmapper

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

//nolint:paralleltest // One hardware device verifies cancellation and handle cleanup.
func TestHardwareGoBakeCancellationInvalidatesOnlyItsDevice(t *testing.T) {
	if os.Getenv("KARTY_LIGHTMAPPER_GPU_TEST") != "1" {
		t.Skip("opt-in hardware compute")
	}

	prepared, receivers := samplePoolTailFixture(t)

	scene, err := PackGPUScene(prepared)
	if err != nil {
		t.Fatal(err)
	}

	tile, err := PackGPUTile(prepared, receivers, Options{Samples: 1 << 24, Outputs: OutputRGB | OutputRNM, Bounces: 3, Seed: 29})
	if err != nil {
		t.Fatal(err)
	}
	// Admit exactly one sample lane so this job cannot finish before cancellation.
	budget := uint64(76+len(scene.Data)+len(tile.Data)) + gpuPathHeaderBytes + uint64(tile.Count)*(520+56*4*2)

	worker, err := NewGPUWorker(t.Context(), GPUWorkerOptions{MemoryBytes: budget})
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = worker.Close() })

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()

	started := time.Now()

	raw, err := worker.Run(ctx, scene, tile)
	if !errors.Is(err, context.DeadlineExceeded) || raw != nil || worker.SceneUploads() != 1 {
		t.Fatal("canceled bake", len(raw), err, worker.SceneUploads())
	}

	if time.Since(started) > 2*time.Second {
		t.Fatal("cancellation did not release the active generation promptly")
	}

	if _, err = worker.Run(t.Context(), scene, tile); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("canceled generation reused", err)
	}
	// A separate device still works after releasing the canceled generation.
	replacement, err := NewGPUWorker(t.Context(), GPUWorkerOptions{MemoryBytes: 128 << 20})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = replacement.Close() }()

	tile.Options.Samples = 1
	if _, err = replacement.Run(t.Context(), scene, tile); err != nil {
		t.Fatal(err)
	}
}

//nolint:paralleltest // Invalid shaders must return a captured error without terminating the CLI process.
func TestHardwareGoBakeRejectsInvalidShaderWithoutAbortingProcess(t *testing.T) {
	if os.Getenv("KARTY_LIGHTMAPPER_GPU_TEST") != "1" {
		t.Skip("opt-in hardware compute")
	}

	source, err := nativeGPUShaderSource()
	if err != nil {
		t.Fatal(err)
	}

	worker, err := newNativeGPUExecutorSource(t.Context(), GPUWorkerOptions{MemoryBytes: 128 << 20}, string(source)+"\ninvalid WGSL syntax")
	if worker != nil || err == nil {
		if worker != nil {
			_ = worker.Close()
		}

		t.Fatal("invalid shader admitted", err)
	}
}
