//go:build !js

package lightmapper

import (
	"context"
	"math"
	"testing"
)

func gpuGlobalLightQualification(t *testing.T, ctx context.Context, worker *GPUWorker) {
	t.Helper()

	for _, emitter := range []Emitter{
		{ID: 2, Kind: Directional, Direction: Vec3{0, 0, -1}, Intensity: Vec3{100, 100, 100}},
		{ID: 2, Kind: Sun, Direction: Vec3{0, 0, -1}, Radius: 1e-4, Intensity: Vec3{1e8, 1e8, 1e8}},
	} {
		source := testCoreScene()
		source.Emitters = []Emitter{{ID: 1, Kind: Point, Position: Vec3{0, 0, 2}, Intensity: Vec3{.01, .01, .01}}, emitter}

		prepared, err := PrepareScene(source)
		if err != nil {
			t.Fatal(err)
		}

		options := Options{Samples: 128, Outputs: OutputRGB | OutputRNM, Seed: 87}
		receivers := []Receiver{testPointReceiver()}

		gpu, err := worker.Bake(ctx, prepared, receivers, options)
		if err != nil {
			t.Fatal(err)
		}

		cpu, err := BakeReference(ctx, prepared, receivers, options)
		if err != nil {
			t.Fatal(err)
		}

		gpuCompare(t, gpu, cpu, .002)

		expected := float32(100/lightPi) + .0025/lightPi
		if emitter.Kind == Sun {
			expected = 1 + .0025/lightPi
		}

		if math.Abs(float64(gpu.Estimates[0].Statistics[0][0]-expected)) > float64(expected)*.04 {
			t.Fatal("global/tiny-Sun energy", emitter.Kind, gpu.Estimates[0].Statistics[0], expected)
		}
	}

	t.Log("dominant globals retain probability mass; radius 1e-4 sun retains finite energy/PDF")
}
