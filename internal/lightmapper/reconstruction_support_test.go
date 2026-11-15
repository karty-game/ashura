//go:build !js

package lightmapper

import (
	"bytes"
	"cmp"
	"reflect"
	"slices"
	"testing"
)

func preparedSupportFixture() ([]Receiver, []ReconstructionSample, []ReconstructionEdge, ReconstructionAdjacency) {
	receivers, halo, _, _ := reconstructionGraphFixture()
	for index := range receivers {
		halo[index].Space = "world"
		receivers[index].Position = halo[index].Position
		receivers[index].Normal = halo[index].Normal
		receivers[index].Tangent = halo[index].Tangent
		receivers[index].Handedness = halo[index].Handedness
		receivers[index].Side = halo[index].Side
	}

	receivers[2].Tangent, halo[2].Tangent = Vec3{}, Vec3{}
	owners := []uint32{0, 1, 2, 3, 4, 5}
	slices.SortFunc(owners, func(first, second uint32) int { return cmp.Compare(receivers[first].ID, receivers[second].ID) })
	rows := ReconstructionAdjacency{Owners: owners, Offsets: []uint32{0}}

	var edges []ReconstructionEdge

	for _, from := range owners {
		for _, to := range owners {
			if from != to {
				rows.Targets = append(rows.Targets, to)
				edges = append(edges, ReconstructionEdge{From: receivers[from].ID, To: receivers[to].ID, Weight: 1, Visible: true})
			}
		}
		//nolint:gosec // G115: this fixture has six receivers and thirty targets.
		rows.Offsets = append(rows.Offsets, uint32(len(rows.Targets)))
	}

	return receivers, halo, edges, rows
}

func supportFixtureEstimates(halo []ReconstructionSample) []Estimate {
	estimates := make([]Estimate, len(halo))
	for index, sample := range halo {
		estimates[index] = Estimate{
			ReceiverID: sample.ID, Count: 32, VarianceValid: true, Statistics: [4]Vec3(sample.Mean), Variance: [4]Vec3(sample.Variance),
			Direct: sample.Direct, Environment: sample.Environment, Reflected: sample.Reflected,
		}
	}

	return estimates
}

func TestPreparedReconstructionSupportPacketsMatchWeightedReference(t *testing.T) {
	t.Parallel()

	receivers, halo, edges, rows := preparedSupportFixture()

	support, err := PrepareReconstructionSupport(receivers, rows)
	if err != nil {
		t.Fatal(err)
	}

	partition, reference, err := referencePartitionedGraph(t.Context(), receivers, halo, edges)
	if err != nil {
		t.Fatal(err)
	}

	selectedReceivers, selectedHalo, err := support.prepareSamples("world", receivers, supportFixtureEstimates(halo))
	if err != nil || !reflect.DeepEqual(selectedReceivers, partition.Receivers) || !reflect.DeepEqual(selectedHalo, partition.Halo) ||
		!slices.Equal(support.original, partition.OriginalIndices) {
		t.Fatal("prepared support changed receiver/frame ownership", err)
	}

	graph := &reconstructionGraph{
		halo: selectedHalo, starts: support.starts, offsets: support.offsets, unitLinks: support.targets,
		marks: make([]uint32, len(selectedHalo)), local: make([]uint32, len(selectedHalo)),
	}
	device := GPUDevice{MaxBufferSize: 1 << 30, MaxStorageBufferBindingSize: 1 << 30, MaxStorageBuffersPerShaderStage: 4}

	for _, strength := range []float32{0, .5, 1} {
		options := ReconstructionOptions{Radius: 1, NormalCos: .9, TangentCos: .9, NoiseScale: 64, Strength: strength, MaximumHalo: 5}
		for first := 0; first < len(selectedReceivers); {
			last, selected, batchErr := graph.nextBatch(first, 256, options, device, 1<<30)
			if batchErr != nil {
				t.Fatal(batchErr)
			}

			referenceLast, referenceSelected, referenceErr := reference.nextBatch(first, 256, options, device, 1<<30)
			if referenceErr != nil || last != referenceLast || !slices.Equal(selected, referenceSelected) ||
				!bytes.Equal(
					graph.pack(first, last, selected, options).Data,
					reference.pack(first, referenceLast, referenceSelected, options).Data,
				) {
				t.Fatal("prepared CSR changed canonical halo or compute packet", referenceErr)
			}

			first = last
		}
	}
}

func TestPreparedReconstructionSupportOwnsInputAndRejectsMutation(t *testing.T) {
	t.Parallel()

	receivers, halo, _, rows := preparedSupportFixture()

	support, err := PrepareReconstructionSupport(receivers, rows)
	if err != nil {
		t.Fatal(err)
	}

	targets, offsets, starts := slices.Clone(support.targets), slices.Clone(support.offsets), slices.Clone(support.starts)

	clear(rows.Owners)
	clear(rows.Targets)
	clear(rows.Offsets)

	if !slices.Equal(targets, support.targets) || !slices.Equal(offsets, support.offsets) || !slices.Equal(starts, support.starts) {
		t.Fatal("sealed support retained caller-owned adjacency arrays")
	}

	estimates := supportFixtureEstimates(halo)
	if _, _, err = support.prepareSamples("world", receivers, estimates); err != nil {
		t.Fatal("adjacency mutation changed admitted support", err)
	}

	for _, mutate := range []func(*Receiver){
		func(receiver *Receiver) { receiver.Position[0]++ },
		func(receiver *Receiver) { receiver.Normal[0]++ },
		func(receiver *Receiver) { receiver.Tangent[1]++ },
		func(receiver *Receiver) { receiver.Handedness++ },
		func(receiver *Receiver) { receiver.Side++ },
	} {
		changed := slices.Clone(receivers)
		mutate(&changed[0])

		if _, _, err = support.prepareSamples("world", changed, estimates); err == nil {
			t.Fatal("changed static receiver guide reused sealed support")
		}
	}

	receivers[0].ID++
	if _, _, err = support.prepareSamples("world", receivers, estimates); err == nil {
		t.Fatal("changed receiver order/identity reused sealed support")
	}
}

func TestPreparedReconstructionSupportCompleteAdmission(t *testing.T) {
	t.Parallel()

	for _, mutate := range []func([]Receiver, *ReconstructionAdjacency){
		func(receivers []Receiver, _ *ReconstructionAdjacency) { receivers[1].ID = receivers[0].ID },
		func(_ []Receiver, rows *ReconstructionAdjacency) { rows.Owners[0] = 100 },
		func(_ []Receiver, rows *ReconstructionAdjacency) { rows.Owners[1] = rows.Owners[0] },
		func(_ []Receiver, rows *ReconstructionAdjacency) { rows.Targets[0] = 100 },
		func(_ []Receiver, rows *ReconstructionAdjacency) { rows.Targets[0] = rows.Owners[0] },
		func(_ []Receiver, rows *ReconstructionAdjacency) { rows.Targets[1] = rows.Targets[0] },
		func(_ []Receiver, rows *ReconstructionAdjacency) { rows.Offsets[0] = 1 },
		func(_ []Receiver, rows *ReconstructionAdjacency) { rows.Offsets[1] = 0 },
		func(_ []Receiver, rows *ReconstructionAdjacency) { rows.Offsets[len(rows.Owners)]-- },
		func(_ []Receiver, rows *ReconstructionAdjacency) { rows.Isolated = []uint32{rows.Owners[0]} },
		func(_ []Receiver, rows *ReconstructionAdjacency) { rows.Isolated = []uint32{100} },
	} {
		receivers, _, _, rows := preparedSupportFixture()
		mutate(receivers, &rows)

		if _, err := PrepareReconstructionSupport(receivers, rows); err == nil {
			t.Fatal("invalid row identity, count, duplicate, bound or isolation admitted")
		}
	}
}
