//go:build !js

package lightmapper

import (
	"reflect"
	"testing"
)

func TestReconstructionPreservesZeroAverageTangentEstimates(t *testing.T) {
	t.Parallel()

	receivers := []Receiver{{ID: 1}, {ID: 2}, {ID: 3}}
	halo := []ReconstructionSample{
		{ID: 1, Normal: Vec3{0, 1, 0}, Tangent: Vec3{1, 0, 0}, Handedness: 1, Samples: 256, Finalized: true},
		{ID: 2, Normal: Vec3{0, 1, 0}, Handedness: 1, Samples: 256, Finalized: true},
		{ID: 3, Normal: Vec3{0, 1, 0}, Tangent: Vec3{1, 0, 0}, Handedness: 1, Samples: 256, Finalized: true},
	}
	edges := []ReconstructionEdge{
		{From: 1, To: 2, Weight: 1, Visible: true},
		{From: 2, To: 3, Weight: 1, Visible: true},
		{From: 1, To: 3, Weight: 1, Visible: true},
	}

	options := ReconstructionOptions{Radius: 1, NormalCos: 0, TangentCos: 0, NoiseScale: 1, Strength: 1, MaximumHalo: 3}
	if reconstructionWeight(halo[0], halo[1], options) != 0 || reconstructionWeight(halo[1], halo[2], options) != 0 {
		t.Fatal("zero tangent contributed to a reconstruction kernel")
	}

	if _, err := PackGPUReconstruction([]uint64{2}, halo[1:2], nil, options); err == nil {
		t.Fatal("raw halo admission accepted a non-unit frame")
	}

	partition, err := partitionReconstructionFrames(receivers, halo, edges)
	if err != nil || !reflect.DeepEqual(partition.OriginalIndices, []int{0, 2}) || len(partition.Edges) != 1 ||
		partition.Edges[0] != edges[2] {
		t.Fatal(partition, err)
	}

	original := []Estimate{
		{ReceiverID: 1, Count: 256},
		{
			ReceiverID:         2,
			Count:              256,
			Statistics:         [4]Vec3{{.1, .2, .3}},
			SecondMoments:      [4]Vec3{{2, 3, 4}},
			Variance:           [4]Vec3{{5, 6, 7}},
			VarianceValid:      true,
			SecondMomentsValid: true,
			RNM:                [3]Vec3{{.3, .5, .7}},
			Direct:             Vec3{.1, .3, .5},
		},
		{ReceiverID: 3, Count: 256},
	}
	filtered := []Estimate{{ReceiverID: 1, Count: 256, Direct: Vec3{1, 2, 3}}, {ReceiverID: 3, Count: 256}}

	result, err := mergeReconstructionFrames(original, filtered, partition.OriginalIndices)
	if err != nil || result[1] != original[1] || result[0] != filtered[0] || result[2] != filtered[1] {
		t.Fatal("lost finalized estimate bits/order", result, err)
	}

	halo[1].Variance[0][0] = -1
	if _, err := partitionReconstructionFrames(receivers, halo, edges); err == nil {
		t.Fatal("zero frame bypassed numeric admission")
	}
}
