//go:build !js

package lightmapper

import (
	"bytes"
	"cmp"
	"errors"
	"reflect"
	"slices"
	"testing"
)

func reconstructionGraphFixture() ([]Receiver, []ReconstructionSample, []ReconstructionEdge, ReconstructionOptions) {
	receivers := []Receiver{{ID: 51}, {ID: 7}, {ID: 100}, {ID: 8}, {ID: 3}, {ID: 29}}

	halo := make([]ReconstructionSample, len(receivers))
	for index, receiver := range receivers {
		halo[index] = ReconstructionSample{
			ID:         receiver.ID,
			Space:      "world",
			Normal:     Vec3{0, 0, 1},
			Tangent:    Vec3{1, 0, 0},
			Handedness: 1,
			Side:       1,
			Samples:    32,
			Finalized:  true,
		}
		halo[index].Position = Vec3{float32(index) * .1, 0, 0}
		halo[index].Mean[0] = Vec3{float32(index + 1), 2, 3}
		halo[index].Variance[0] = Vec3{1, 1, 1}
	}

	halo[4].Space = "independent overlapping world"
	edges := []ReconstructionEdge{
		{From: 51, To: 100, Weight: 1, Visible: true}, {From: 51, To: 7, Weight: .5, Visible: true},
		{From: 7, To: 100, Weight: 1, Visible: true}, {From: 100, To: 7, Weight: 1, Visible: true},
		{From: 8, To: 3, Weight: .25, Visible: false}, {From: 3, To: 29, Weight: 1, Visible: true},
		{From: 29, To: 3, Weight: 1, Visible: true}, {From: 29, To: 29, Weight: 1, Visible: true},
	}
	options := ReconstructionOptions{
		Radius:         1,
		PlaneTolerance: .01,
		NormalCos:      .9,
		TangentCos:     .9,
		NoiseScale:     2,
		Strength:       1,
		MaximumHalo:    3,
	}

	return receivers, halo, edges, options
}

func TestIndexedReconstructionPacketsMatchReferenceAcrossCompleteHalos(t *testing.T) {
	t.Parallel()

	receivers, halo, edges, options := reconstructionGraphFixture()

	graph, err := newReconstructionGraph(receivers, halo, edges)
	if err != nil {
		t.Fatal(err)
	}

	index, links, err := reconstructionTileLinks(receivers, edges)
	if err != nil {
		t.Fatal(err)
	}

	device := GPUDevice{MaxBufferSize: 1 << 30, MaxStorageBufferBindingSize: 1 << 30, MaxStorageBuffersPerShaderStage: 4}
	batches := 0

	for start := 0; start < len(receivers); {
		end, selected, batchErr := graph.nextBatch(start, 256, options, device, 1<<30)
		if batchErr != nil {
			t.Fatal(batchErr)
		}

		ids, localHalo, localEdges := reconstructionLocalHalo(receivers[start:end], index, links, halo)

		want, packErr := PackGPUReconstruction(ids, localHalo, localEdges, options)
		if packErr != nil {
			t.Fatal(packErr)
		}

		got := graph.pack(start, end, selected, options)
		if got.Count != want.Count || got.Mode != want.Mode || got.Options != want.Options || !bytes.Equal(got.Data, want.Data) {
			t.Fatalf("changed exact reconstruction packet for interiors %d..%d", start, end)
		}

		batches++
		start = end
	}

	if batches < 2 {
		t.Fatal("fixture did not exercise cross-tile halos")
	}
}

func TestIndexedReconstructionAdmissionNeverTruncatesNeighborSupport(t *testing.T) {
	t.Parallel()

	receivers, halo, edges, options := reconstructionGraphFixture()

	graph, err := newReconstructionGraph(receivers, halo, edges)
	if err != nil {
		t.Fatal(err)
	}

	device := GPUDevice{MaxBufferSize: 1 << 30, MaxStorageBufferBindingSize: 1 << 30, MaxStorageBuffersPerShaderStage: 4}

	options.MaximumHalo = 2
	if _, _, err = graph.nextBatch(0, 256, options, device, 1<<30); !errors.Is(err, ErrBounds) {
		t.Fatal("silently truncated required third halo receiver", err)
	}

	options.MaximumHalo = 3
	if _, _, err = graph.nextBatch(0, 256, options, device, 256); !errors.Is(err, ErrBounds) {
		t.Fatal("allocated a packet outside complete live budget", err)
	}

	end, selected, err := graph.nextBatch(0, 256, options, device, 1<<30)
	if err != nil || end == 0 || !reflect.DeepEqual(selected, []uint32{0, 1, 2}) {
		t.Fatal("failed admission poisoned the next exact batch", end, selected, err)
	}
}

func TestIndexedReconstructionRejectsDuplicateAndInvalidEdgesBeforeBatching(t *testing.T) {
	t.Parallel()

	receivers, halo, edges, _ := reconstructionGraphFixture()
	for _, bad := range []ReconstructionEdge{{From: 51, To: 7, Weight: 1, Visible: false}, {From: 51, To: 999, Weight: 1, Visible: true}, {From: 51, To: 8, Weight: -1, Visible: true}} {
		invalid := append(append([]ReconstructionEdge(nil), edges...), bad)
		if _, err := newReconstructionGraph(receivers, halo, invalid); err == nil {
			t.Fatal("accepted malformed complete graph", bad)
		}
	}
}

func TestCanonicalReconstructionGraphMatchesArbitraryInputOrder(t *testing.T) {
	t.Parallel()

	receivers, halo, edges, options := reconstructionGraphFixture()

	arbitrary, err := newReconstructionGraph(receivers, halo, edges)
	if err != nil {
		t.Fatal(err)
	}

	slices.SortFunc(edges, func(first, second ReconstructionEdge) int {
		if first.From != second.From {
			return cmp.Compare(first.From, second.From)
		}

		return cmp.Compare(first.To, second.To)
	})

	canonical, err := newReconstructionGraph(receivers, halo, edges)
	if err != nil {
		t.Fatal(err)
	}

	device := GPUDevice{MaxBufferSize: 1 << 30, MaxStorageBufferBindingSize: 1 << 30, MaxStorageBuffersPerShaderStage: 4}

	for start := 0; start < len(receivers); {
		end, selected, err := arbitrary.nextBatch(start, 256, options, device, 1<<30)
		if err != nil {
			t.Fatal(err)
		}

		reference := arbitrary.pack(start, end, selected, options)

		canonicalEnd, canonicalSelected, err := canonical.nextBatch(start, 256, options, device, 1<<30)
		if err != nil || canonicalEnd != end {
			t.Fatal("canonical fast path changed halo admission", end, canonicalEnd, err)
		}

		indexed := canonical.pack(start, canonicalEnd, canonicalSelected, options)
		if !bytes.Equal(indexed.Data, reference.Data) {
			t.Fatal("canonical source groups changed exact packet", start, end)
		}

		start = end
	}
}
