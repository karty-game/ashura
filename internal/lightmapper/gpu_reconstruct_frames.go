//go:build !js

package lightmapper

import "fmt"

type reconstructionFramePartition struct {
	Receivers       []Receiver
	Halo            []ReconstructionSample
	Edges           []ReconstructionEdge
	OriginalIndices []int
}

// A zero averaged tangent can result from opposing valid UV0 patch frames.
// Every incident kernel weight is exactly zero because its tangent-dot factor
// is zero. Preserve that finalized estimate; do not invent a filtering frame.
//
//nolint:cyclop,funlen // Validate original identities, immutable estimates and every edge before preserving zero-weight frames.
func partitionReconstructionFrames(
	receivers []Receiver,
	halo []ReconstructionSample,
	edges []ReconstructionEdge,
) (reconstructionFramePartition, error) {
	var partition reconstructionFramePartition
	if len(receivers) != len(halo) {
		return partition, ErrInvalid
	}

	zero := make(map[uint64]bool)

	for _, sample := range halo {
		if sample.Tangent == (Vec3{}) {
			zero[sample.ID] = true
		}
	}

	if len(zero) == 0 {
		partition.Receivers = receivers

		return partition, nil
	}

	remaining := len(receivers) - len(zero)
	partition.Receivers = make([]Receiver, 0, remaining)
	partition.Halo = make([]ReconstructionSample, 0, remaining)
	partition.OriginalIndices = make([]int, 0, remaining)
	partition.Edges = make([]ReconstructionEdge, 0, len(edges))

	indices := make(map[uint64]bool, len(receivers))
	for index, receiver := range receivers {
		sample := halo[index]
		if indices[receiver.ID] || receiver.ID != sample.ID || !sample.Finalized || sample.Samples < 2 ||
			!reconstructionValidValues(
				sample,
			) || !positiveVec(sample.Direct) || !positiveVec(sample.Environment) || !positiveVec(sample.Reflected) {
			return reconstructionFramePartition{}, fmt.Errorf(
				"reconstruction receiver %d numeric/identity admission: %w",
				receiver.ID,
				errReconstruction,
			)
		}

		indices[receiver.ID] = true
		if zero[receiver.ID] {
			continue
		}

		if !reconstructionValidSample(sample) {
			return reconstructionFramePartition{}, fmt.Errorf(
				"reconstruction receiver %d non-unit filtering frame: %w",
				receiver.ID,
				errReconstruction,
			)
		}

		partition.Receivers = append(partition.Receivers, receiver)
		partition.Halo = append(partition.Halo, sample)
		partition.OriginalIndices = append(partition.OriginalIndices, index)
	}

	omitted := make(map[[2]uint64]bool)

	for _, edge := range edges {
		if !indices[edge.From] || !indices[edge.To] || !receiverFinite(edge.Weight) || edge.Weight <= 0 {
			return reconstructionFramePartition{}, fmt.Errorf(
				"reconstruction edge %d/%d admission: %w",
				edge.From,
				edge.To,
				errReconstruction,
			)
		}

		if zero[edge.From] || zero[edge.To] {
			key := [2]uint64{edge.From, edge.To}
			if omitted[key] {
				return reconstructionFramePartition{}, errReconstruction
			}

			omitted[key] = true

			continue
		}

		partition.Edges = append(partition.Edges, edge)
	}

	return partition, nil
}

func mergeReconstructionFrames(original, reconstructed []Estimate, indices []int) ([]Estimate, error) {
	if len(reconstructed) != len(indices) {
		return nil, errReconstruction
	}

	result := append([]Estimate(nil), original...)
	for index, target := range indices {
		if target < 0 || target >= len(result) || reconstructed[index].ReceiverID != original[target].ReceiverID {
			return nil, errReconstruction
		}

		result[target] = reconstructed[index]
	}

	return result, nil
}
