//go:build !js

package lightmapper

import (
	"bytes"
	"cmp"
	"context"
	"math"
	"math/rand/v2"
	"reflect"
	"slices"
	"testing"
)

func referencePartitionedGraph(
	ctx context.Context,
	receivers []Receiver,
	halo []ReconstructionSample,
	edges []ReconstructionEdge,
) (reconstructionFramePartition, *reconstructionGraph, error) {
	partition, err := partitionReconstructionFrames(receivers, halo, edges)
	if err != nil {
		return partition, nil, err
	}

	if len(partition.Receivers) == len(receivers) {
		if len(receivers) == 0 {
			return partition, nil, ErrInvalid
		}

		graph, err := newReconstructionGraph(receivers, halo, edges)

		return partition, graph, err
	}

	if err = ctx.Err(); err != nil {
		return partition, nil, err
	}

	if len(partition.Receivers) == 0 {
		return partition, nil, nil
	}

	graph, err := newReconstructionGraph(partition.Receivers, partition.Halo, partition.Edges)

	return partition, graph, err
}

func indexedPartitionedGraph(
	ctx context.Context,
	receivers []Receiver,
	halo []ReconstructionSample,
	edges []ReconstructionEdge,
) (reconstructionFramePartition, *reconstructionGraph, error) {
	partition, graph, err := partitionReconstructionGraph(receivers, halo, edges)
	if err != nil {
		return partition, nil, err
	}

	if len(partition.Receivers) == len(receivers) {
		if len(receivers) == 0 {
			return partition, nil, ErrInvalid
		}

		graph, err = newReconstructionGraph(receivers, halo, edges)

		return partition, graph, err
	}

	if err = ctx.Err(); err != nil {
		return partition, nil, err
	}

	if len(partition.Receivers) == 0 {
		return partition, nil, nil
	}

	if graph == nil {
		graph, err = newReconstructionGraph(partition.Receivers, partition.Halo, partition.Edges)
	} else {
		err = admitPartitionedReconstructionGraph(graph)
	}

	return partition, graph, err
}

func comparePartitionedGraph(
	t *testing.T,
	ctx context.Context,
	receivers []Receiver,
	halo []ReconstructionSample,
	edges []ReconstructionEdge,
) {
	t.Helper()

	referencePartition, referenceGraph, referenceErr := referencePartitionedGraph(ctx, receivers, halo, edges)

	candidatePartition, candidateGraph, candidateErr := indexedPartitionedGraph(ctx, receivers, halo, edges)
	if (referenceErr == nil) != (candidateErr == nil) || referenceErr != nil && referenceErr.Error() != candidateErr.Error() {
		t.Fatalf("admission precedence changed: reference %v candidate %v", referenceErr, candidateErr)
	}

	if referenceErr != nil {
		return
	}

	if !reflect.DeepEqual(referencePartition.Receivers, candidatePartition.Receivers) ||
		!reflect.DeepEqual(referencePartition.Halo, candidatePartition.Halo) ||
		!reflect.DeepEqual(referencePartition.OriginalIndices, candidatePartition.OriginalIndices) {
		t.Fatal("frame ownership changed")
	}

	if !reflect.DeepEqual(referenceGraph, candidateGraph) {
		t.Fatal("CSR ownership/ordering/weights differ")
	}

	if referenceGraph == nil {
		return
	}

	options := ReconstructionOptions{Radius: 1, NormalCos: 0, TangentCos: 0, NoiseScale: 1, Strength: 1, MaximumHalo: 100}

	device := GPUDevice{MaxBufferSize: 1 << 30, MaxStorageBufferBindingSize: 1 << 30, MaxStorageBuffersPerShaderStage: 4}
	for first := 0; first < len(referenceGraph.halo); {
		last, selection, err := referenceGraph.nextBatch(first, 256, options, device, 1<<30)
		if err != nil {
			t.Fatal(err)
		}

		candidateLast, candidateSelection, candidateBatchErr := candidateGraph.nextBatch(first, 256, options, device, 1<<30)
		if candidateBatchErr != nil || last != candidateLast || !slices.Equal(selection, candidateSelection) {
			t.Fatal("halo/interior admission changed")
		}

		if !bytes.Equal(
			referenceGraph.pack(first, last, selection, options).Data,
			candidateGraph.pack(first, candidateLast, candidateSelection, options).Data,
		) {
			t.Fatal("packet bytes changed")
		}

		first = last
	}
}

type partitionGraphFixture struct {
	receivers []Receiver
	halo      []ReconstructionSample
	edges     []ReconstructionEdge
}

func partitionGraphMutations() map[string]func(*partitionGraphFixture) {
	return map[string]func(*partitionGraphFixture){
		"nil":   func(fixture *partitionGraphFixture) { fixture.receivers = nil; fixture.halo = nil; fixture.edges = nil },
		"count": func(fixture *partitionGraphFixture) { fixture.halo = fixture.halo[:1] },
		"duplicateReceiver": func(fixture *partitionGraphFixture) {
			fixture.receivers[2].ID = fixture.receivers[0].ID
			fixture.halo[2].ID = fixture.receivers[0].ID
		},
		"invalidRetainedSamples":    func(fixture *partitionGraphFixture) { fixture.halo[0].Samples = 1 << 32 },
		"largeOmittedSamples":       func(fixture *partitionGraphFixture) { fixture.halo[1].Samples = 1 << 32 },
		"invalidOmittedVariance":    func(fixture *partitionGraphFixture) { fixture.halo[1].Variance[0][0] = -1 },
		"duplicateOmittedEdge":      func(fixture *partitionGraphFixture) { fixture.edges = append(fixture.edges, fixture.edges[0]) },
		"invalidWeight":             func(fixture *partitionGraphFixture) { fixture.edges[0].Weight = float32(math.NaN()) },
		"unknownBeforeLargeSamples": func(fixture *partitionGraphFixture) { fixture.edges[0].From = 0; fixture.halo[0].Samples = 1 << 32 },
		"allZero": func(fixture *partitionGraphFixture) {
			for index := range fixture.halo {
				fixture.halo[index].Tangent = Vec3{}
			}
		},
	}
}

func partitionGraphEdgeOrder(first, second ReconstructionEdge) int {
	if order := cmp.Compare(first.From, second.From); order != 0 {
		return order
	}

	return cmp.Compare(first.To, second.To)
}

func TestPartitionedReconstructionGraphDifferentialAdmission(t *testing.T) {
	t.Parallel()

	receivers, halo, edges, _ := reconstructionGraphFixture()
	halo[1].Tangent = Vec3{}

	slices.SortFunc(edges, partitionGraphEdgeOrder)
	comparePartitionedGraph(t, t.Context(), receivers, halo, edges)

	for name, change := range partitionGraphMutations() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			fixture := partitionGraphFixture{slices.Clone(receivers), slices.Clone(halo), slices.Clone(edges)}
			change(&fixture)
			comparePartitionedGraph(t, t.Context(), fixture.receivers, fixture.halo, fixture.edges)
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			comparePartitionedGraph(t, ctx, fixture.receivers, fixture.halo, fixture.edges)
		})
	}
}

func TestPartitionedReconstructionGraphDifferentialCorpus(t *testing.T) {
	t.Parallel()

	//nolint:gosec // G404: fixed-seed randomness constructs reproducible test data, without security use.
	random := rand.New(rand.NewPCG(42, 17))

	const corpusCases = 300

	for iteration := range corpusCases {
		receivers, halo, _, _ := reconstructionGraphFixture()
		edges := make([]ReconstructionEdge, 0, len(receivers)*len(receivers))

		for index := range halo {
			if random.IntN(3) == 0 {
				halo[index].Tangent = Vec3{}
			}
		}

		for _, from := range receivers {
			for _, to := range receivers {
				if random.IntN(3) == 0 {
					edges = append(
						edges,
						ReconstructionEdge{From: from.ID, To: to.ID, Weight: float32(random.IntN(10)+1) / 10, Visible: random.IntN(2) == 0},
					)
				}
			}
		}

		slices.SortFunc(edges, partitionGraphEdgeOrder)

		switch iteration % 8 {
		case 0:
			if len(edges) > 0 {
				edges[0].Weight = -1
			}
		case 1:
			if len(edges) > 0 {
				edges = append(edges, edges[0])
			}
		case 2:
			if len(edges) > 0 {
				edges[0].To = 999
			}
		case 3:
			halo[1].Samples = 1 << 32
		case 4:
			halo[3].Handedness = 0
		case 5:
			slices.Reverse(edges)
		}

		comparePartitionedGraph(t, t.Context(), receivers, halo, edges)
	}
}
