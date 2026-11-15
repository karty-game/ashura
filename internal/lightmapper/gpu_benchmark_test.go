//go:build !js

package lightmapper

import (
	"context"
	"fmt"
	"math"
	"os"
	"testing"
	"time"
)

// This opt-in measurement compares the actual hardware hierarchy and exhaustive
// GPU kernels at equal measured error against an independent exhaustive CPU sum.
// It isolates direct point-light selection; it does not claim full-scene bake or
// mobile performance. Device startup, pipeline compilation and scene upload are
// warmed outside each measurement. Job submission/readback remain in wall time.
//
//nolint:paralleltest // One hardware executor measures the two algorithms serially.
func TestHardwareGPUEqualErrorManyLights(t *testing.T) {
	if os.Getenv("KARTY_LIGHTMAPPER_GPU_BENCHMARK") != "1" {
		t.Skip("set KARTY_LIGHTMAPPER_GPU_BENCHMARK=1 with hardware GPU access")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Minute)
	defer cancel()

	worker, err := NewGPUWorker(ctx, GPUWorkerOptions{MemoryBytes: 128 << 20})
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = worker.Close() })
	t.Log("hardware", worker.Device().Adapter)

	for _, count := range []int{1, 64, 1024, 16384} {
		for _, sparse := range []bool{false, true} {
			t.Run(fmt.Sprintf("lights%d/sparse%v", count, sparse), func(t *testing.T) {
				gpuMeasureEqualError(t, ctx, worker, count, sparse)
			})
		}
	}
}

func gpuMeasureEqualError(t *testing.T, ctx context.Context, worker *GPUWorker, count int, sparse bool) {
	t.Helper()

	scene, receivers := gpuBenchmarkScene(t, count, sparse)
	expected := gpuBenchmarkReference(scene, receivers)

	options := Options{Samples: 1, Outputs: OutputRGB | OutputRNM, Seed: 19}
	if _, err := worker.Bake(ctx, scene, receivers, options); err != nil {
		t.Fatal(err)
	}

	options.Samples = 4
	options.ExactLights = true

	referenceTime, referenceError := gpuBenchmarkMeasure(t, ctx, worker, scene, receivers, options, expected)
	if referenceError > .001 {
		t.Fatal("exhaustive GPU kernel does not match independent point-light sum", referenceError)
	}

	options.ExactLights = false

	const targetError = .02

	for _, samples := range []uint32{16, 64, 256, 1024, 4096} {
		options.Samples = samples
		elapsed, relativeError := gpuBenchmarkMeasure(t, ctx, worker, scene, receivers, options, expected)
		t.Logf("hierarchy samples=%d relative-RMSE=%.6f wall=%s", samples, relativeError, elapsed)

		if relativeError <= targetError {
			t.Logf(
				"equal-error target=%.3f hierarchy_samples=%d hierarchy=%s exhaustive_samples=4 exhaustive=%s exhaustive_relative-RMSE=%.6f speed_ratio=%.3f",
				targetError,
				samples,
				elapsed,
				referenceTime,
				referenceError,
				float64(referenceTime)/float64(elapsed),
			)

			return
		}
	}

	t.Fatal("hierarchy did not reach 2% measured relative RMSE within the explicit 4096-sample benchmark ladder")
}

func gpuBenchmarkMeasure(
	t *testing.T,
	ctx context.Context,
	worker *GPUWorker,
	scene *PreparedScene,
	receivers []Receiver,
	options Options,
	expected []Moments,
) (time.Duration, float64) {
	t.Helper()

	var (
		elapsed                        time.Duration
		squaredError, squaredReference float64
	)

	for _, seed := range []uint32{19, 713} {
		options.Seed = seed
		started := time.Now()
		result, err := worker.Bake(ctx, scene, receivers, options)
		elapsed += time.Since(started)

		if err != nil {
			t.Fatal(err)
		}

		for receiver, estimate := range result.Estimates {
			for kernel := range 4 {
				for channel := range 3 {
					reference := float64(expected[receiver][kernel][channel])
					delta := float64(estimate.Statistics[kernel][channel]) - reference
					squaredError += delta * delta
					squaredReference += reference * reference
				}
			}
		}
	}

	return elapsed / 2, math.Sqrt(squaredError / max(squaredReference, 1e-30))
}

func gpuBenchmarkScene(t *testing.T, count int, sparse bool) (*PreparedScene, []Receiver) {
	t.Helper()

	source := testCoreScene()
	source.Emitters = make([]Emitter, count)

	rows, columns := (count+127)/128, min(count, 128)
	for index := range count {
		coordinateX := float32(index%columns) - float32(columns-1)*.5
		coordinateY := float32(index/columns) - float32(rows-1)*.5

		model, finiteRange, spacing := Physical, float32(0), float32(.01)
		if sparse {
			model, finiteRange, spacing = LegacyRangeV1, 6, 2
		}

		weight := float32(1+(index*13)%31) / float32(count)
		source.Emitters[index] = Emitter{
			ID: uint64(index + 1), Kind: Point, Model: model, Range: finiteRange,
			Position:  Vec3{coordinateX * spacing, coordinateY * spacing, 1.5 + float32(index%7)*.05},
			Intensity: Vec3{weight, weight * .6, weight * .2},
		}
	}

	scene, err := PrepareScene(source)
	if err != nil {
		t.Fatal(err)
	}

	receivers := make([]Receiver, 8)
	for index := range receivers {
		receiver := testPointReceiver()
		receiver.ID = uint64(index + 1)
		receiver.Patches[0].RawTangent = Vec3{1, 0, 0}
		receiver.Patches[0].RawBitangent = Vec3{0, 1, 0}

		receiver.Position = Vec3{float32(index%4) - 1.5, float32(index/4) - .5, 0}
		for vertex := range receiver.Patches[0].Positions {
			receiver.Patches[0].Positions[vertex] = Add(receiver.Patches[0].Positions[vertex], receiver.Position)
		}

		receivers[index] = receiver
	}

	return scene, receivers
}

// The tiny receiver patches differ from their center by at most 1e-5 units;
// the exhaustive hardware baseline checks that this approximation is below .1%.
// Float64 summation here is independent of both GPU Welford and hierarchy PMFs.
func gpuBenchmarkReference(scene *PreparedScene, receivers []Receiver) []Moments {
	result := make([]Moments, len(receivers))
	for receiver, surface := range receivers {
		var moments [4][3]float64

		for index := range scene.source.Emitters {
			sample := scene.SampleEmitter(surface.Position, uint32(index), .5, .5)
			if sample.Direction[2] <= 0 {
				continue
			}

			kernels := CalibrationKernels(sample.Direction)

			for kernel := range 4 {
				for channel := range 3 {
					moments[kernel][channel] += float64(sample.Radiance[channel]) * float64(kernels[kernel])
				}
			}
		}

		for kernel := range 4 {
			for channel := range 3 {
				result[receiver][kernel][channel] = float32(moments[kernel][channel])
			}
		}
	}

	return result
}
