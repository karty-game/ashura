//go:build !js

package lightmapper

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"os"
	"testing"
	"time"
)

func automaticReconstructionFixture(t *testing.T) ([]Receiver, []Estimate, ReconstructionAdjacency) {
	t.Helper()

	const count = 128

	receivers := make([]Receiver, count)
	estimates := make([]Estimate, count)
	rows := ReconstructionAdjacency{Offsets: []uint32{0}, Isolated: []uint32{17}}

	for index := range receivers {
		receivers[index] = Receiver{
			ID:         uint64(index + 1),
			Position:   Vec3{float32(index) * .01, 0, 0},
			Normal:     Vec3{0, 0, 1},
			Tangent:    Vec3{1, 0, 0},
			Handedness: 1,
			Side:       1,
		}
		kernels := CalibrationKernels(Normalize(Vec3{float32(index%7) - 3, float32(index%5) - 2, 1}))
		estimate := Estimate{ReceiverID: receivers[index].ID, Count: 32, VarianceValid: true, SecondMomentsValid: true}

		for kernel := range 4 {
			for channel := range 3 {
				mean := kernels[kernel] * float32(channel+1)
				estimate.Statistics[kernel][channel] = mean
				estimate.Variance[kernel][channel] = .1
				estimate.SecondMoments[kernel][channel] = mean*mean + .1*31/32
			}
		}

		estimate.Direct = Scale(estimate.Statistics[0], .3)
		estimate.Environment = Scale(estimate.Statistics[0], .3)
		estimate.Reflected = Scale(estimate.Statistics[0], .4)

		rnm, err := ProjectRNM(Moments(estimate.Statistics))
		if err != nil {
			t.Fatal(err)
		}

		estimate.RNM = rnm

		estimates[index] = estimate
		if index == 17 {
			continue
		}

		rows.Owners = append(rows.Owners, uint32(index))
		if index > 0 && index-1 != 17 {
			rows.Targets = append(rows.Targets, uint32(index-1))
		}

		if index+1 < count && index+1 != 17 {
			rows.Targets = append(rows.Targets, uint32(index+1))
		}
		//nolint:gosec // G115: this 128-receiver fixture has at most two links per owner.
		rows.Offsets = append(rows.Offsets, uint32(len(rows.Targets)))
	}

	receivers[23].Tangent = Vec3{}

	return receivers, estimates, rows
}

//nolint:paralleltest,musttag // One serial hardware worker compares every existing Estimate field through independent JSON encoding, without introducing a JSON wire API.
func TestHardwareGPUAutomaticReconstructionHaloByteParity(t *testing.T) {
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
	scene := gpuFixtureScene(t, testCoreScene())
	receivers, estimates, rows := automaticReconstructionFixture(t)

	support, err := PrepareReconstructionSupport(receivers, rows)
	if err != nil {
		t.Fatal(err)
	}

	options := ReconstructionOptions{Radius: 2, PlaneTolerance: .01, NormalCos: .9, TangentCos: .9, NoiseScale: 64, Strength: 1}

	var expected []byte

	for _, limit := range []uint32{3, 7, 0, 128} {
		options.MaximumHalo = limit

		result, err := worker.ReconstructPrepared(ctx, scene, receivers, estimates, support, options)
		if err != nil {
			t.Fatal(limit, err)
		}

		encoded, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}

		if expected == nil {
			expected = encoded
		} else if !bytes.Equal(expected, encoded) {
			t.Fatal("halo scheduling changed encoded field bits, counts or ownership", limit)
		}

		if result[23] != estimates[23] {
			t.Fatal("zero frame lost original finalized estimate", limit)
		}

		for _, value := range result {
			if value.Count != 32 || math.IsNaN(float64(value.Statistics[0][0])) {
				t.Fatal("invalid finalized output", limit)
			}
		}
	}

	t.Logf("adapter=%s receivers=%d caps=3/7/auto/128; all encoded estimate fields byte-identical", worker.Device().Adapter, len(receivers))
}
