//go:build !js

package lightmapper

import (
	"cmp"
	"math"
	"slices"
)

const (
	gpuReconstructionHeaderWords   = 32
	gpuReconstructionEdgeWords     = 2
	gpuReconstructionBatchOverhead = 76
)

type reconstructionLink struct {
	to      uint32
	weight  float32
	visible bool
}

type reconstructionGraph struct {
	halo         []ReconstructionSample
	offsets      []uint32
	starts       []uint32
	links        []reconstructionLink
	unitLinks    []uint32
	marks, local []uint32
	epoch        uint32
	selected     []uint32
}

// newReconstructionGraph validates immutable input once and stores adjacency as
// contiguous index records. Per-tile admission and packing never rebuild ID maps
// or copy/sort the world edge graph.
func newReconstructionGraph(receivers []Receiver, halo []ReconstructionSample, edges []ReconstructionEdge) (*reconstructionGraph, error) {
	if len(receivers) == 0 || len(receivers) != len(halo) || uint64(len(halo)) > math.MaxUint32 || uint64(len(edges)) > math.MaxUint32 {
		return nil, errReconstruction
	}

	graph := &reconstructionGraph{
		halo:    halo,
		offsets: make([]uint32, len(halo)+1),
		marks:   make([]uint32, len(halo)),
		local:   make([]uint32, len(halo)),
	}
	graph.starts = make([]uint32, len(halo))

	indices := make(map[uint64]uint32, len(halo))
	for index, sample := range halo {
		if _, exists := indices[sample.ID]; exists || sample.ID != receivers[index].ID || !sample.Finalized || sample.Samples < 2 ||
			sample.Samples > math.MaxUint32 ||
			!reconstructionValidSample(sample) ||
			!positiveVec(sample.Direct) ||
			!positiveVec(sample.Environment) ||
			!positiveVec(sample.Reflected) {
			return nil, errReconstruction
		}

		indices[sample.ID] = uint32(index)
	}

	if reconstructionEdgesCanonical(edges) {
		if err := graph.loadCanonicalEdges(indices, edges); err != nil {
			return nil, err
		}

		return graph, nil
	}

	if err := graph.loadArbitraryEdges(indices, edges); err != nil {
		return nil, err
	}

	return graph, nil
}

func (graph *reconstructionGraph) loadArbitraryEdges(indices map[uint64]uint32, edges []ReconstructionEdge) error {
	for _, edge := range edges {
		from, fromFound := indices[edge.From]

		_, toFound := indices[edge.To]
		if !fromFound || !toFound || !receiverFinite(edge.Weight) || edge.Weight <= 0 {
			return errReconstruction
		}

		graph.offsets[from+1]++
	}

	for index := 1; index < len(graph.offsets); index++ {
		graph.offsets[index] += graph.offsets[index-1]
	}

	graph.links = make([]reconstructionLink, len(edges))
	copy(graph.starts, graph.offsets[:len(graph.halo)])
	next := slices.Clone(graph.starts)

	for _, edge := range edges {
		from, to := indices[edge.From], indices[edge.To]
		graph.links[next[from]] = reconstructionLink{to: to, weight: edge.Weight, visible: edge.Visible}
		next[from]++
	}

	for owner := range graph.halo {
		links := graph.ownerLinks(uint32(owner))
		slices.SortFunc(links, func(first, second reconstructionLink) int {
			return cmp.Compare(graph.halo[first.to].ID, graph.halo[second.to].ID)
		})

		for index := 1; index < len(links); index++ {
			if links[index-1].to == links[index].to {
				return errReconstruction
			}
		}
	}

	return nil
}

func (graph *reconstructionGraph) selectReceiver(owner uint32) {
	if graph.marks[owner] != graph.epoch {
		graph.marks[owner] = graph.epoch
		graph.selected = append(graph.selected, owner)
	}

	if graph.unitLinks != nil {
		for _, target := range graph.ownerTargets(owner) {
			if graph.marks[target] != graph.epoch {
				graph.marks[target] = graph.epoch
				graph.selected = append(graph.selected, target)
			}
		}

		return
	}

	for _, edge := range graph.ownerLinks(owner) {
		if graph.marks[edge.to] != graph.epoch {
			graph.marks[edge.to] = graph.epoch
			graph.selected = append(graph.selected, edge.to)
		}
	}
}

// nextBatch greedily admits distinct halo nodes before packet allocation. An
// overflowing receiver is rolled back and retried in the next batch, retaining
// every edge; already admitted interiors are never recursively copied/sorted.
//
//nolint:gosec // G115: the admitted graph has at most MaxUint32 receivers and a batch is a positive contiguous subrange.
func (graph *reconstructionGraph) nextBatch(
	start int,
	sceneBytes uint64,
	options ReconstructionOptions,
	device GPUDevice,
	budget uint64,
) (int, []uint32, error) {
	graph.epoch++
	if graph.epoch == 0 {
		clear(graph.marks)
		graph.epoch = 1
	}

	graph.selected = graph.selected[:0]

	var links uint64

	end := start
	for end < len(graph.halo) {
		previous := len(graph.selected)
		graph.selectReceiver(uint32(end))
		candidateLinks := links + uint64(graph.offsets[end+1]-graph.offsets[end])
		count := uint64(end - start + 1)
		words := uint64(
			gpuReconstructionHeaderWords,
		) + uint64(
			len(graph.selected),
		)*gpuReconstructionStride + count + candidateLinks*gpuReconstructionEdgeWords
		bytes := words * gpuWordBytes
		output := count * gpuResultWords * gpuWordBytes

		fits := reconstructionHaloFits(uint64(len(graph.selected)), options) && bytes <= gpuMaximumBytes && bytes <= device.MaxBufferSize &&
			bytes <= device.MaxStorageBufferBindingSize &&
			output <= device.MaxBufferSize &&
			output <= device.MaxStorageBufferBindingSize &&
			sceneBytes+bytes+gpuPathMinimumBytes+2*output+gpuReconstructionBatchOverhead <= budget
		if !fits {
			for _, owner := range graph.selected[previous:] {
				graph.marks[owner] = 0
			}

			graph.selected = graph.selected[:previous]

			break
		}

		links = candidateLinks
		end++
	}

	if end == start {
		return start, nil, ErrBounds
	}

	slices.Sort(graph.selected)

	return end, graph.selected, nil
}

func (graph *reconstructionGraph) ownerLinks(owner uint32) []reconstructionLink {
	start := graph.starts[owner]

	return graph.links[start : start+graph.offsets[owner+1]-graph.offsets[owner]]
}

func (graph *reconstructionGraph) ownerTargets(owner uint32) []uint32 {
	start := graph.starts[owner]

	return graph.unitLinks[start : start+graph.offsets[owner+1]-graph.offsets[owner]]
}

func reconstructionEdgesCanonical(edges []ReconstructionEdge) bool {
	for index := 1; index < len(edges); index++ {
		previous, current := edges[index-1], edges[index]
		if previous.From > current.From || previous.From == current.From && previous.To >= current.To {
			return false
		}
	}

	return true
}

// Prepared support already carries canonical contiguous source/destination
// groups. Validate and consume that ordering directly instead of hashing every
// source twice and sorting millions of already ordered destinations again.
func (graph *reconstructionGraph) loadCanonicalEdges(indices map[uint64]uint32, edges []ReconstructionEdge) error {
	graph.links = make([]reconstructionLink, len(edges))

	var source uint32

	var previous uint64
	for index, edge := range edges {
		if index == 0 || previous != edge.From {
			var found bool

			source, found = indices[edge.From]
			if !found {
				return errReconstruction
			}

			graph.starts[source] = uint32(index)
			previous = edge.From
		}

		target, found := indices[edge.To]
		if !found || !receiverFinite(edge.Weight) || edge.Weight <= 0 {
			return errReconstruction
		}

		graph.offsets[source+1]++
		graph.links[index] = reconstructionLink{to: target, weight: edge.Weight, visible: edge.Visible}
	}

	for index := 1; index < len(graph.offsets); index++ {
		graph.offsets[index] += graph.offsets[index-1]
	}

	return nil
}
