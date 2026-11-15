//go:build !js

package lightmapper

import (
	"fmt"
	"math"
)

const omittedReconstructionFrame = uint32(math.MaxUint32)

// partitionReconstructionGraph admits every original receiver and edge before
// producing a CSR graph. Canonical support shares one dense identity index across
// frame filtering and graph construction, without copying the complete edge list.
// Arbitrary edge order retains the original admission and duplicate checks.
func partitionReconstructionGraph(
	receivers []Receiver,
	halo []ReconstructionSample,
	edges []ReconstructionEdge,
) (reconstructionFramePartition, *reconstructionGraph, error) {
	var partition reconstructionFramePartition

	if len(receivers) != len(halo) {
		return partition, nil, ErrInvalid
	}

	zero := make(map[uint64]bool)

	for _, sample := range halo {
		if sample.Tangent == (Vec3{}) {
			zero[sample.ID] = true
		}
	}

	if len(zero) == 0 || uint64(len(receivers)) > math.MaxUint32 || !reconstructionEdgesCanonical(edges) {
		partition, err := partitionReconstructionFrames(receivers, halo, edges)

		return partition, nil, err
	}

	partition, indices, err := indexReconstructionFrames(receivers, halo, zero)
	if err != nil {
		return reconstructionFramePartition{}, nil, err
	}

	graph, err := canonicalPartitionGraph(partition.Halo, indices, edges)
	if err != nil {
		return reconstructionFramePartition{}, nil, err
	}

	return partition, graph, nil
}

// indexReconstructionFrames preserves the original receiver admission order.
// The sentinel is outside the admitted dense index range, including when every
// original receiver has a zero tangent and no receiver survives for filtering.
func indexReconstructionFrames(
	receivers []Receiver,
	halo []ReconstructionSample,
	zero map[uint64]bool,
) (reconstructionFramePartition, map[uint64]uint32, error) {
	remaining := len(receivers) - len(zero)
	partition := reconstructionFramePartition{
		Receivers:       make([]Receiver, 0, remaining),
		Halo:            make([]ReconstructionSample, 0, remaining),
		OriginalIndices: make([]int, 0, remaining),
	}

	indices := make(map[uint64]uint32, len(receivers))

	for index, receiver := range receivers {
		sample := halo[index]

		_, duplicate := indices[receiver.ID]
		if duplicate || receiver.ID != sample.ID || !sample.Finalized || sample.Samples < 2 || !reconstructionValidValues(sample) ||
			!positiveVec(sample.Direct) ||
			!positiveVec(sample.Environment) ||
			!positiveVec(sample.Reflected) {
			return reconstructionFramePartition{}, nil, fmt.Errorf(
				"reconstruction receiver %d numeric/identity admission: %w",
				receiver.ID,
				errReconstruction,
			)
		}

		if zero[receiver.ID] {
			indices[receiver.ID] = omittedReconstructionFrame

			continue
		}

		if !reconstructionValidSample(sample) {
			return reconstructionFramePartition{}, nil, fmt.Errorf(
				"reconstruction receiver %d non-unit filtering frame: %w",
				receiver.ID,
				errReconstruction,
			)
		}

		//nolint:gosec // G115: partitionReconstructionGraph admits at most MaxUint32 original receivers; a retained index is strictly smaller.
		indices[receiver.ID] = uint32(len(partition.Receivers))
		partition.Receivers = append(partition.Receivers, receiver)
		partition.Halo = append(partition.Halo, sample)
		partition.OriginalIndices = append(partition.OriginalIndices, index)
	}

	return partition, indices, nil
}

// canonicalPartitionGraph admits omitted links too, preserving the original
// first invalid edge. Strict source/destination ordering already proves every
// pair unique; duplicate and arbitrary inputs use partitionReconstructionFrames.
func canonicalPartitionGraph(
	halo []ReconstructionSample,
	indices map[uint64]uint32,
	edges []ReconstructionEdge,
) (*reconstructionGraph, error) {
	graph := &reconstructionGraph{
		halo:    halo,
		offsets: make([]uint32, len(halo)+1),
		starts:  make([]uint32, len(halo)),
		links:   make([]reconstructionLink, 0, len(edges)),
	}

	var (
		source      uint32
		sourceFound bool
		previous    uint64
	)

	for index, edge := range edges {
		if index == 0 || previous != edge.From {
			source, sourceFound = indices[edge.From]
			previous = edge.From
		}

		target, targetFound := indices[edge.To]
		if !sourceFound || !targetFound || !receiverFinite(edge.Weight) || edge.Weight <= 0 {
			return nil, fmt.Errorf("reconstruction edge %d/%d admission: %w", edge.From, edge.To, errReconstruction)
		}

		if source == omittedReconstructionFrame || target == omittedReconstructionFrame {
			continue
		}

		if graph.offsets[source+1] == 0 {
			//nolint:gosec // G115: admitPartitionedReconstructionGraph rejects any CSR exceeding MaxUint32 links before consumption.
			graph.starts[source] = uint32(len(graph.links))
		}

		graph.offsets[source+1]++
		graph.links = append(graph.links, reconstructionLink{to: target, weight: edge.Weight, visible: edge.Visible})
	}

	return graph, nil
}

// admitPartitionedReconstructionGraph retains graph-only size and sample-count
// admission after the caller's cancellation check. Earlier frame/edge admission
// has the same error precedence as the original partition followed by CSR build.
func admitPartitionedReconstructionGraph(graph *reconstructionGraph) error {
	if len(graph.halo) == 0 || uint64(len(graph.halo)) > math.MaxUint32 || uint64(len(graph.links)) > math.MaxUint32 {
		return errReconstruction
	}

	for _, sample := range graph.halo {
		if sample.Samples > math.MaxUint32 {
			return errReconstruction
		}
	}

	for index := 1; index < len(graph.offsets); index++ {
		graph.offsets[index] += graph.offsets[index-1]
	}

	graph.marks = make([]uint32, len(graph.halo))
	graph.local = make([]uint32, len(graph.halo))

	return nil
}
