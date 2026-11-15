//go:build !js

package lightmapper

import "math"

// pack retains reconstruction ABI @1 and canonical destination-ID edge order.
// All numeric/identity validation happened once when the graph was admitted.
//
//nolint:gosec // G115: complete 128 MiB batch admission bounds all record counts, offsets and positive interior differences.
func (graph *reconstructionGraph) pack(start, end int, selected []uint32, options ReconstructionOptions) GPUTile {
	linkCount := uint64(graph.offsets[end] - graph.offsets[start])
	words := uint64(
		gpuReconstructionHeaderWords,
	) + uint64(
		len(selected),
	)*gpuReconstructionStride + uint64(
		end-start,
	) + linkCount*gpuReconstructionEdgeWords
	packet := make(gpuWords, gpuReconstructionHeaderWords+len(selected)*gpuReconstructionStride, words)
	packet[0], packet[1], packet[2], packet[3] = 0x4b4c5251, 1, uint32(end-start), uint32(len(selected))
	packet[4], packet[5] = 32, uint32(len(packet))
	spaces := make(map[string]uint32)

	for local, global := range selected {
		graph.local[global] = uint32(local)
	}

	for local, global := range selected {
		sample := graph.halo[global]

		space, found := spaces[sample.Space]
		if !found {
			space = uint32(len(spaces))
			spaces[sample.Space] = space
		}

		packet = graph.packSample(packet, local, global, space, start, end)
	}

	packet[6] = uint32(len(packet))
	for global := start; global < end; global++ {
		packet = append(packet, graph.local[global])
	}

	for index, value := range []float32{options.Radius, options.PlaneTolerance, options.NormalCos, options.TangentCos, options.NoiseScale, options.Strength} {
		packet[8+index] = math.Float32bits(value)
	}

	return GPUTile{
		Data:    packet.bytes(),
		Count:   uint32(end - start),
		Options: Options{Outputs: OutputRGB | OutputRNM, Samples: 1},
		Mode:    gpuModeReconstruct,
	}
}

//nolint:gosec // G115: ID/Side preserve raw wire bits; graph admission bounds Samples to uint32 before packing.
func (graph *reconstructionGraph) packSample(packet gpuWords, local int, global, space uint32, start, end int) gpuWords {
	sample := graph.halo[global]
	offset := gpuReconstructionHeaderWords + local*gpuReconstructionStride
	packet[offset], packet[offset+1] = uint32(sample.ID), uint32(sample.ID>>32)
	packet[offset+2], packet[offset+3] = space, uint32(sample.Side)
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
	packet[offset+50] = uint32((len(packet) - int(packet[5])) / 2)

	if int(global) >= start && int(global) < end {
		if graph.unitLinks != nil {
			for _, target := range graph.ownerTargets(global) {
				packet = append(packet, graph.local[target], math.Float32bits(1))
				packet[offset+51]++
			}

			return packet
		}

		for _, edge := range graph.ownerLinks(global) {
			if !edge.visible || edge.to == global {
				continue
			}

			packet = append(packet, graph.local[edge.to], math.Float32bits(edge.weight))
			packet[offset+51]++
		}
	}

	return packet
}
