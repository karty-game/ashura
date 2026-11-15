//go:build !js

package lightmapper

import (
	"bytes"
	"context"
	"os"
	"testing"
	"time"
)

//nolint:paralleltest // This hardware regression owns one native GPU worker.
func TestHardwareGPUWideVerificationOutputLayout(t *testing.T) {
	if os.Getenv("KARTY_LIGHTMAPPER_GPU_TEST") != "1" {
		t.Skip("set KARTY_LIGHTMAPPER_GPU_TEST=1 with hardware GPU access")
	}

	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()

	worker, err := NewGPUWorker(ctx, GPUWorkerOptions{MemoryBytes: 128 << 20})
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = worker.Close() })

	scene := gpuFixtureScene(t, Scene{Environment: Environment{Sky: Vec3{.2, .15, .1}, Ground: Vec3{.2, .15, .1}}})
	receivers := []Receiver{gpuFixtureReceiver(1, 0), gpuFixtureReceiver(2, 1), gpuFixtureReceiver(3, 2)}

	normals := make([][]Vec3, len(receivers))
	for index := range normals {
		normals[index] = make([]Vec3, 64)
		for probe := range normals[index] {
			normals[index][probe] = Vec3{0, 0, 1}
			if probe%2 != 0 {
				normals[index][probe] = Vec3{.8660254, 0, .5}
			}
		}
	}

	// At 64 probes the response stride exceeds path scratch capacity. Its
	// complete AoS wire must use the original output, not serialized paths.
	options := Options{Outputs: OutputRGB | OutputRNM, Samples: 64, Seed: 713}

	packed, err := PackGPUScene(scene)
	if err != nil {
		t.Fatal(err)
	}

	batch := runGPUOutputLayout(t, ctx, worker, scene, packed, receivers, normals, options)
	split := make([]byte, 0, len(batch))

	for index := range receivers {
		result := runGPUOutputLayout(t, ctx, worker, scene, packed, receivers[index:index+1], normals[index:index+1], options)
		split = append(split, result...)
	}

	if !bytes.Equal(batch, split) {
		t.Fatal("complete verification result changed across receiver tile shapes")
	}
}

func runGPUOutputLayout(
	t *testing.T,
	ctx context.Context,
	worker *GPUWorker,
	scene *PreparedScene,
	packed GPUScene,
	receivers []Receiver,
	normals [][]Vec3,
	options Options,
) []byte {
	t.Helper()

	tile, err := PackGPUVerification(scene, receivers, options, normals)
	if err != nil {
		t.Fatal(err)
	}

	raw, err := worker.Run(ctx, packed, tile)
	if err != nil {
		t.Fatal(err)
	}

	results, err := DecodeGPUResponses(raw, receivers, normals, tile.ProbeCount)
	if err != nil {
		t.Fatal(err)
	}

	for index, result := range results {
		if result.Count != options.Samples || len(result.Means) != len(normals[index]) {
			t.Fatal("incomplete verification response", result.Count, len(result.Means))
		}
	}

	return raw
}
