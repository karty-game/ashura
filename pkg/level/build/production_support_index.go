//go:build !js

package levelbuild

import (
	"runtime"
	"sync"

	bake "github.com/karty-game/karty-webgpu/pkg/lightmap/bake"
)

// Destination identity lookup, complete edge admission and dense conversion
// share one pass. Workers read immutable inputs and own disjoint output ranges.
func indexSupportTargets(targets []uint32, edges []bake.ReconstructionEdge, indices map[uint64]uint32, active []bool) error {
	count := min(len(edges), runtime.GOMAXPROCS(0))
	if count <= 1 {
		return indexSupportTargetRange(targets, edges, indices, active, 0, len(edges))
	}

	errors := make([]error, count)

	var workers sync.WaitGroup

	for index := range errors {
		workers.Go(func() {
			first, last := len(edges)*index/count, len(edges)*(index+1)/count
			errors[index] = indexSupportTargetRange(targets, edges, indices, active, first, last)
		})
	}

	workers.Wait()

	for _, err := range errors {
		if err != nil {
			return err
		}
	}

	return nil
}

func indexSupportTargetRange(
	targets []uint32, edges []bake.ReconstructionEdge, indices map[uint64]uint32, active []bool, first, last int,
) error {
	for index := first; index < last; index++ {
		edge := edges[index]
		if edge.From == edge.To || edge.Weight != 1 || !edge.Visible || index > 0 && !supportEdgeBefore(edges[index-1], edge) {
			return ErrInvalid
		}

		if index == first || edge.From != edges[index-1].From {
			from, found := indices[edge.From]
			if !found || !active[from] {
				return ErrInvalid
			}
		}

		to, found := indices[edge.To]
		if !found || !active[to] {
			return ErrInvalid
		}

		targets[index] = to
	}

	return nil
}
