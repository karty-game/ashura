//go:build !js

package lightmapper

import (
	"context"
	"math"
	"os"
	"testing"
	"time"
)

//nolint:paralleltest // One actual device checks the production projection kernel against the independent float64 solver.
func TestHardwareGPUCalibratedSimplexProjection(t *testing.T) {
	if os.Getenv("KARTY_LIGHTMAPPER_GPU_TEST") != "1" {
		t.Skip("set KARTY_LIGHTMAPPER_GPU_TEST=1 with hardware GPU access")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	worker, err := NewGPUWorker(ctx, GPUWorkerOptions{
		MemoryBytes: 128 << 20,
	})
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = worker.Close() })
	t.Log("hardware", worker.Device().Adapter)

	scene := gpuFixtureScene(t, Scene{})
	scales := []float32{1e-20, 1, 1e20}
	receivers := make([]Receiver, 0, len(scales)*67)
	halo := make([]ReconstructionSample, 0, len(scales)*67)

	for _, scale := range scales {
		for index := range 67 {
			receiverID := uint64(len(receivers) + 1)
			receiver := gpuFixtureReceiver(receiverID, 0)

			var moments Moments

			for channel := range 3 {
				moments[0][channel] = scale * float32(index%5) / 4
				for basis := range 3 {
					moments[basis+1][channel] = scale * float32((index*(basis+3)+channel*7)%23) / 11
				}
			}

			receivers = append(receivers, receiver)
			halo = append(halo, ReconstructionSample{
				ID: receiverID, Space: "projection", Position: receiver.Position, Normal: receiver.Normal,
				Tangent: receiver.Tangent, Handedness: receiver.Handedness, Side: receiver.Side,
				Mean: moments, Samples: 32, Finalized: true,
			})
		}
	}

	options := ReconstructionOptions{
		Radius: .125, PlaneTolerance: .015, NormalCos: .95, TangentCos: .95,
		NoiseScale: 64, Strength: 1, MaximumHalo: 1024,
	}

	got, err := worker.ReconstructHalo(ctx, scene, receivers, halo, nil, options)
	if err != nil {
		t.Fatal(err)
	}

	for index, sample := range halo {
		want, projectErr := ProjectRNM(sample.Mean)
		if projectErr != nil {
			t.Fatal(projectErr)
		}

		for channel := range 3 {
			tolerance := float64(max(float32(1e-20), 3*sample.Mean[0][channel])) * 4e-5
			for basis := range 3 {
				if math.Abs(float64(got[index].RNM[basis][channel]-want[basis][channel])) > tolerance {
					t.Fatalf("case%d basis%d channel%d: GPU%g CPU%g", index, basis, channel,
						got[index].RNM[basis][channel], want[basis][channel])
				}
			}
		}
	}
}
