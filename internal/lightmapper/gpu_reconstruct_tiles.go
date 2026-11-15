//go:build !js

package lightmapper

import (
	"context"
	"sort"
)

func (worker *GPUWorker) reconstructTiles(
	ctx context.Context, scene *PreparedScene, receivers []Receiver, halo []ReconstructionSample,
	edges []ReconstructionEdge, options ReconstructionOptions,
) ([]Estimate, error) {
	if worker == nil || ctx == nil || len(receivers) == 0 || len(receivers) != len(halo) {
		return nil, ErrInvalid
	}

	graph, err := newReconstructionGraph(receivers, halo, edges)
	if err != nil {
		return nil, err
	}

	return worker.reconstructGraphTiles(ctx, scene, receivers, graph, options)
}

func (worker *GPUWorker) reconstructGraphTiles(
	ctx context.Context,
	scene *PreparedScene,
	receivers []Receiver,
	graph *reconstructionGraph,
	options ReconstructionOptions,
) ([]Estimate, error) {
	packet, err := worker.scenePacket(scene)
	if err != nil {
		return nil, err
	}

	result := make([]Estimate, len(receivers))
	device := worker.Device()

	for start := 0; start < len(receivers); {
		if err = ctx.Err(); err != nil {
			return nil, err
		}

		end, selected, batchErr := graph.nextBatch(start, uint64(len(packet.Data)), options, device, worker.options.MemoryBytes)
		if batchErr != nil {
			return nil, batchErr
		}

		tile := graph.pack(start, end, selected, options)

		data, runErr := worker.Run(ctx, packet, tile)
		if runErr != nil {
			return nil, runErr
		}

		if decodeErr := decodeGPUEstimatesInto(data, receivers[start:end], result[start:end]); decodeErr != nil {
			return nil, decodeErr
		}

		start = end
	}

	return result, nil
}

func reconstructionLocalHalo(
	batch []Receiver,
	index map[uint64]int,
	links map[uint64][]ReconstructionEdge,
	halo []ReconstructionSample,
) ([]uint64, []ReconstructionSample, []ReconstructionEdge) {
	selected := make(map[int]bool)

	var localEdges []ReconstructionEdge

	ids := make([]uint64, len(batch))
	for i, receiver := range batch {
		ids[i] = receiver.ID

		selected[index[receiver.ID]] = true
		for _, edge := range links[receiver.ID] {
			selected[index[edge.To]] = true
			localEdges = append(localEdges, edge)
		}
	}

	selectedIndices := make([]int, 0, len(selected))
	for i := range selected {
		selectedIndices = append(selectedIndices, i)
	}

	sort.Ints(selectedIndices)

	localHalo := make([]ReconstructionSample, len(selectedIndices))
	for i, global := range selectedIndices {
		localHalo[i] = halo[global]
	}

	return ids, localHalo, localEdges
}

func reconstructionTileLinks(receivers []Receiver, edges []ReconstructionEdge) (map[uint64]int, map[uint64][]ReconstructionEdge, error) {
	index := make(map[uint64]int, len(receivers))
	links := make(map[uint64][]ReconstructionEdge)

	for i, receiver := range receivers {
		if _, exists := index[receiver.ID]; exists {
			return nil, nil, ErrInvalid
		}

		index[receiver.ID] = i
	}

	for _, edge := range edges {
		if _, exists := index[edge.From]; !exists {
			return nil, nil, ErrInvalid
		}

		if _, exists := index[edge.To]; !exists {
			return nil, nil, ErrInvalid
		}

		links[edge.From] = append(links[edge.From], edge)
	}

	return index, links, nil
}
