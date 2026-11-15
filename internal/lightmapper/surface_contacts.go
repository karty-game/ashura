package lightmapper

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
)

type surfaceContactPair struct {
	first, second uint32
}

// Candidate geometry is independent once the spatial index is immutable. Test
// it in parallel, then admit only actual contacts in the original segment/cell
// order. Ambiguity isolation and diagnostics retain their deterministic input;
// neither overlapping contact intervals nor partial boundary contacts are lost.
func (grid *surfaceGrid) connectContacts(ctx context.Context) error {
	return grid.connectContactsWithWorkers(ctx, runtime.GOMAXPROCS(0))
}

//nolint:gosec // G115: complete segment admission bounds each parallel range below MaxUint32.
func (grid *surfaceGrid) connectContactsWithWorkers(ctx context.Context, requestedWorkers int) error {
	workers := min(requestedWorkers, len(grid.segments))
	parts := make([][]surfaceContactPair, workers)

	var work atomic.Uint64
	work.Store(grid.work)

	var stopped atomic.Bool

	surfaceContactRanges(len(grid.segments), workers, func(_, start, end int) {
		grid.countContactRange(ctx, start, end, &work, &stopped)
	})
	grid.work = work.Load()

	if err := ctx.Err(); err != nil {
		return err
	}

	if stopped.Load() {
		return fmt.Errorf("surface candidate work exceeds limit %d: %w", grid.options.MaximumCandidates, ErrBounds)
	}

	// The complete candidate count is admitted before geometry or contact-buffer
	// allocation. This second scan avoids a contended atomic for every candidate.
	surfaceContactRanges(len(grid.segments), workers, func(worker, start, end int) {
		var contacts []surfaceContactPair

		for index := start; index < end; index++ {
			if ctx.Err() != nil {
				return
			}

			contacts = grid.contactCandidates(uint32(index), contacts)
		}

		parts[worker] = contacts
	})

	if err := ctx.Err(); err != nil {
		return err
	}

	for _, part := range parts {
		for _, pair := range part {
			if err := ctx.Err(); err != nil {
				return err
			}

			if err := grid.connect(pair.first, pair.second); err != nil {
				return err
			}
		}
	}

	return nil
}

//nolint:gosec // G115: complete segment admission bounds each parallel range below MaxUint32.
func (grid *surfaceGrid) countContactRange(ctx context.Context, start, end int, work *atomic.Uint64, stopped *atomic.Bool) {
	var pending uint64

	for index := start; index < end; index++ {
		if stopped.Load() || ctx.Err() != nil {
			return
		}

		pending += grid.contactCandidateCount(uint32(index))

		const batch = 1024
		if pending >= batch {
			if work.Add(pending) > grid.options.MaximumCandidates {
				stopped.Store(true)

				return
			}

			pending = 0
		}
	}

	if work.Add(pending) > grid.options.MaximumCandidates {
		stopped.Store(true)
	}
}

func surfaceContactRanges(count, workers int, run func(worker, start, end int)) {
	var group sync.WaitGroup

	for worker := range workers {
		width, extra := count/workers, count%workers
		start := worker*width + min(worker, extra)

		end := start + width
		if worker < extra {
			end++
		}

		group.Go(func() { run(worker, start, end) })
	}

	group.Wait()
}

func (grid *surfaceGrid) contactCandidateCount(index uint32) uint64 {
	segment := grid.segments[index]
	low := grid.minima[index]
	_, high, _ := surfaceCellRange(segment, grid.cellSize, grid.tolerances[index], grid.options.MaximumCandidates)

	var count uint64

	for x := low[0]; x <= high[0]; x++ {
		for y := low[1]; y <= high[1]; y++ {
			for z := low[2]; z <= high[2]; z++ {
				cell := surfaceCell{x, y, z}
				for _, other := range grid.cells[cell] {
					if other >= index {
						break
					}

					if grid.segments[other].owner != segment.owner && surfaceFirstSharedCell(cell, low, grid.minima[other]) {
						count++
						if count > grid.options.MaximumCandidates {
							return count
						}
					}
				}
			}
		}
	}

	return count
}

func (grid *surfaceGrid) contactCandidates(index uint32, output []surfaceContactPair) []surfaceContactPair {
	segment := grid.segments[index]
	low := grid.minima[index]
	// Insertion already admitted the complete span and its cell count.
	_, high, _ := surfaceCellRange(segment, grid.cellSize, grid.tolerances[index], grid.options.MaximumCandidates)
	for x := low[0]; x <= high[0]; x++ {
		for y := low[1]; y <= high[1]; y++ {
			for z := low[2]; z <= high[2]; z++ {
				cell := surfaceCell{x, y, z}
				for _, other := range grid.cells[cell] {
					if other >= index {
						break
					}

					if grid.segments[other].owner == segment.owner || !surfaceFirstSharedCell(cell, low, grid.minima[other]) {
						continue
					}

					if grid.contactEligible(index, other) {
						output = append(output, surfaceContactPair{first: index, second: other})
					}
				}
			}
		}
	}

	return output
}

func (grid *surfaceGrid) contactEligible(index, otherIndex uint32) bool {
	segment, other := grid.segments[index], grid.segments[otherIndex]
	tolerance := max(grid.tolerances[index], grid.tolerances[otherIndex])

	return surfaceBoundsOverlap(segment, other, tolerance) &&
		grid.receivers[segment.owner].Side == grid.receivers[other.owner].Side &&
		(surfaceSupportEligible(grid.receivers[segment.owner], grid.receivers[other.owner], grid.options.Support) ||
			surfaceSupportEligible(grid.receivers[other.owner], grid.receivers[segment.owner], grid.options.Support)) &&
		surfaceSegmentsTouch(segment, other, tolerance)
}
