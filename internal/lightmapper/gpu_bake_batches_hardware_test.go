//go:build !js

package lightmapper

import (
	"context"
	"os"
	"reflect"
	"testing"
	"time"
)

//nolint:paralleltest // One hardware worker compares independent and batched fixed sampling serially.
func TestHardwareGPUBakeBatchesMatchIndependentJobs(t *testing.T) {
	if os.Getenv("KARTY_LIGHTMAPPER_GPU_TEST") != "1" {
		t.Skip("set KARTY_LIGHTMAPPER_GPU_TEST=1 with hardware GPU access")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()

	worker, err := NewGPUWorker(ctx, GPUWorkerOptions{MemoryBytes: 128 << 20})
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = worker.Close() })
	scene, receivers := gpuBenchmarkScene(t, 64, false)
	batches := []BakeBatch{
		{Receivers: receivers, Options: Options{Samples: 32, Bounces: 3, RouletteDepth: 1, Seed: 29, Outputs: OutputRGB | OutputRNM}},
		{Receivers: receivers[1:4], Options: Options{Samples: 16, Bounces: 2, RouletteDepth: 1, Seed: 71, Outputs: OutputRGB | OutputRNM}},
	}

	actual, err := worker.BakeBatches(ctx, scene, batches)
	if err != nil {
		t.Fatal(err)
	}

	for index, batch := range batches {
		expected, err := worker.Bake(ctx, scene, batch.Receivers, batch.Options)
		if err != nil || !reflect.DeepEqual(actual[index], expected) {
			t.Fatal("batched hardware statistics/provenance differ", index, err)
		}
	}

	t.Logf(
		"adapter=%s pages=%d receivers=%d samples=32/16; complete statistics and result metadata byte-identical",
		worker.Device().Adapter,
		len(batches),
		len(receivers)+3,
	)
}
