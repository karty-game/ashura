//go:build !js

package lightmapper

import (
	"context"
	"math"
	"sort"
)

const gpuReconstructionStride = 52

// PackGPUReconstruction admits immutable finalized halos and explicit surface
// links. GPU output contains only the requested interior in the same order.
//
//nolint:gosec,mnd // G115: complete byte-size admission bounds packet offsets/counts; IDs and Side retain their exact wire bits. Constants describe the versioned 52-word GPU record.
func PackGPUReconstruction(
	interior []uint64,
	halo []ReconstructionSample,
	edges []ReconstructionEdge,
	options ReconstructionOptions,
) (GPUTile, error) {
	if len(interior) == 0 || !reconstructionValidOptions(options) || !reconstructionHaloFits(uint64(len(halo)), options) {
		return GPUTile{}, errReconstruction
	}

	words := uint64(32) + uint64(len(halo))*gpuReconstructionStride + uint64(len(interior)) + uint64(len(edges))*2
	if words*4 > gpuMaximumBytes {
		return GPUTile{}, ErrBounds
	}

	indices, spaces, byFrom, err := reconstructionPacketIndices(halo, edges)
	if err != nil {
		return GPUTile{}, err
	}

	packet := make(gpuWords, 32+len(halo)*gpuReconstructionStride, words)
	packet[0], packet[1], packet[2], packet[3] = 0x4b4c5251, 1, uint32(len(interior)), uint32(len(halo))
	packet[4] = 32
	packet[5] = uint32(len(packet))

	for index, sample := range halo {
		packet, err = encodeReconstructionSample(packet, index, sample, spaces, byFrom[sample.ID], indices)
		if err != nil {
			return GPUTile{}, err
		}
	}

	packet[6] = uint32(len(packet))

	seen := make(map[uint64]bool, len(interior))
	for _, receiverID := range interior {
		index, exists := indices[receiverID]
		if !exists || seen[receiverID] {
			return GPUTile{}, errReconstruction
		}

		seen[receiverID] = true

		packet = append(packet, index)
	}

	for index, value := range []float32{options.Radius, options.PlaneTolerance, options.NormalCos, options.TangentCos, options.NoiseScale, options.Strength} {
		packet[8+index] = math.Float32bits(value)
	}

	return GPUTile{
		Data:    packet.bytes(),
		Count:   uint32(len(interior)),
		Options: Options{Outputs: OutputRGB | OutputRNM, Samples: 1},
		Mode:    "reconstruct",
	}, nil
}

// ReconstructHalo runs surface reconstruction and calibrated projection on the
// admitted GPU. The halo is finalized previous-pass data; current-pass interiors
// are never fed back into this call as neighbors.
func (w *GPUWorker) ReconstructHalo(
	ctx context.Context,
	scene *PreparedScene,
	interior []Receiver,
	halo []ReconstructionSample,
	edges []ReconstructionEdge,
	options ReconstructionOptions,
) ([]Estimate, error) {
	if scene == nil {
		return nil, ErrInvalid
	}

	ids := make([]uint64, len(interior))
	for receiverIndex, receiver := range interior {
		ids[receiverIndex] = receiver.ID
	}

	tile, err := PackGPUReconstruction(ids, halo, edges, options)
	if err != nil {
		return nil, err
	}

	packed, err := w.scenePacket(scene)
	if err != nil {
		return nil, err
	}

	data, err := w.Run(ctx, packed, tile)
	if err != nil {
		return nil, err
	}

	return DecodeGPUEstimates(data, interior)
}

// ReconstructionSamples converts transport outputs into an immutable guided
// halo. Adjacency is deliberately not inferred from coordinate proximity.
func ReconstructionSamples(space string, receivers []Receiver, estimates []Estimate) ([]ReconstructionSample, error) {
	if len(receivers) != len(estimates) {
		return nil, ErrInvalid
	}

	result := make([]ReconstructionSample, len(receivers))
	for receiverIndex, receiver := range receivers {
		var err error

		result[receiverIndex], err = reconstructionSample(space, receiver, estimates[receiverIndex])
		if err != nil {
			return nil, err
		}
	}

	return result, nil
}

func reconstructionSample(space string, receiver Receiver, estimate Estimate) (ReconstructionSample, error) {
	if estimate.ReceiverID != receiver.ID || estimate.Count < 2 || !estimate.VarianceValid {
		return ReconstructionSample{}, ErrInvalid
	}

	sample := ReconstructionSample{
		ID:         receiver.ID,
		Space:      space,
		Position:   receiver.Position,
		Normal:     receiver.Normal,
		Tangent:    receiver.Tangent,
		Handedness: receiver.Handedness,
		Side:       receiver.Side,
		Mean:       Moments(estimate.Statistics),
		Variance:   Moments(estimate.Variance),
		Samples: uint64(
			estimate.Count,
		),
		Finalized:   true,
		Direct:      estimate.Direct,
		Environment: estimate.Environment,
		Reflected:   estimate.Reflected,
	}
	for _, variance := range sample.Variance {
		if !positiveVec(variance) {
			return ReconstructionSample{}, ErrInvalid
		}
	}

	return sample, nil
}

// Reconstruct tiles interiors with their complete immutable neighbor halos.
// Memory bounds change scheduling only; every output keeps the same support.
func (worker *GPUWorker) Reconstruct(
	ctx context.Context,
	scene *PreparedScene,
	receivers []Receiver,
	estimates []Estimate,
	edges []ReconstructionEdge,
	options ReconstructionOptions,
) ([]Estimate, error) {
	if worker == nil || ctx == nil || scene == nil {
		return nil, ErrInvalid
	}

	if !reconstructionValidOptions(options) {
		return nil, errReconstruction
	}

	halo, err := ReconstructionSamples(scene.Source().Space, receivers, estimates)
	if err != nil {
		return nil, err
	}

	partition, indexedGraph, err := partitionReconstructionGraph(receivers, halo, edges)
	if err != nil {
		return nil, err
	}

	if len(partition.Receivers) == len(receivers) {
		return worker.reconstructTiles(ctx, scene, receivers, halo, edges, options)
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var reconstructed []Estimate

	if len(partition.Receivers) > 0 {
		if indexedGraph == nil {
			reconstructed, err = worker.reconstructTiles(ctx, scene, partition.Receivers, partition.Halo, partition.Edges, options)
		} else if err = admitPartitionedReconstructionGraph(indexedGraph); err == nil {
			reconstructed, err = worker.reconstructGraphTiles(ctx, scene, partition.Receivers, indexedGraph, options)
		}

		if err != nil {
			return nil, err
		}
	}

	return mergeReconstructionFrames(estimates, reconstructed, partition.OriginalIndices)
}

//nolint:gosec // G115: packet-size admission bounds every count/index and sample counts are explicitly bounded to uint32.
func reconstructionPacketIndices(
	halo []ReconstructionSample,
	edges []ReconstructionEdge,
) (map[uint64]uint32, map[string]uint32, map[uint64][]ReconstructionEdge, error) {
	indices := make(map[uint64]uint32, len(halo))
	spaces := make(map[string]uint32)

	for receiverIndex, sample := range halo {
		if _, exists := indices[sample.ID]; exists || !sample.Finalized || sample.Samples < 2 || sample.Samples > math.MaxUint32 ||
			!reconstructionValidSample(sample) ||
			!positiveVec(sample.Direct) ||
			!positiveVec(sample.Environment) ||
			!positiveVec(sample.Reflected) {
			return nil, nil, nil, errReconstruction
		}

		indices[sample.ID] = uint32(receiverIndex)
		if _, exists := spaces[sample.Space]; !exists {
			spaces[sample.Space] = uint32(len(spaces))
		}
	}

	byFrom := make(map[uint64][]ReconstructionEdge)

	for _, edge := range edges {
		if !receiverFinite(edge.Weight) || edge.Weight <= 0 {
			return nil, nil, nil, errReconstruction
		}

		if _, exists := indices[edge.From]; !exists {
			return nil, nil, nil, errReconstruction
		}

		if _, exists := indices[edge.To]; !exists {
			return nil, nil, nil, errReconstruction
		}

		byFrom[edge.From] = append(byFrom[edge.From], edge)
	}

	return indices, spaces, byFrom, nil
}

//nolint:gosec,mnd // G115: packet preflight bounds all offsets/counts; ID and Side preserve wire bits. Offsets are the fixed 52-word reconstruction ABI.
func encodeReconstructionSample(
	packet gpuWords,
	index int,
	sample ReconstructionSample,
	spaces map[string]uint32,
	links []ReconstructionEdge,
	indices map[uint64]uint32,
) (gpuWords, error) {
	offset := 32 + index*gpuReconstructionStride
	packet[offset], packet[offset+1] = uint32(sample.ID), uint32(sample.ID>>32)
	packet[offset+2] = spaces[sample.Space]
	packet[offset+3] = uint32(sample.Side)
	packet.vec(offset+4, sample.Position)
	packet[offset+7] = math.Float32bits(sample.Handedness)
	packet.vec(offset+8, sample.Normal)
	packet[offset+11] = uint32(sample.Samples)
	packet.vec(offset+12, sample.Tangent)

	packet[offset+15] = 1
	for kernel := range 4 {
		packet.vec(offset+16+kernel*3, sample.Mean[kernel])
		packet.vec(offset+28+kernel*3, sample.Variance[kernel])
	}

	packet.vec(offset+40, sample.Direct)
	packet.vec(offset+43, sample.Environment)
	packet.vec(offset+46, sample.Reflected)
	sort.Slice(links, func(index, j int) bool { return links[index].To < links[j].To })

	packet[offset+50] = uint32((len(packet) - int(packet[5])) / 2)

	for j, edge := range links {
		if j > 0 && links[j-1].To == edge.To {
			return nil, errReconstruction
		}

		if !edge.Visible || edge.To == edge.From {
			continue
		}

		packet = append(packet, indices[edge.To], math.Float32bits(edge.Weight))
		packet[offset+51]++
	}

	return packet, nil
}
