package lightmapper

import (
	"context"
	"fmt"
	"slices"
)

type surfaceGrid struct {
	receivers     []Receiver
	segments      []surfaceSegment
	options       SurfaceEdgeOptions
	cellSize      float32
	cells         map[surfaceCell][]uint32
	minima        []surfaceCell
	tolerances    []float32
	firstContacts []surfaceContactSlot
	contacts      map[uint32][]surfaceContact
	adjacency     [][]uint32
	work          uint64
	isolated      map[uint32]bool
	diagnostics   []SurfaceContactDiagnostic
}

func (grid *surfaceGrid) insertSegments(ctx context.Context) error {
	for index, segment := range grid.segments {
		if err := ctx.Err(); err != nil {
			return err
		}

		if err := grid.insert(index, segment); err != nil {
			return err
		}
	}

	return nil
}

//nolint:varnamelen,gosec // x/y/z are integer spatial cells. G115: BuildSurfaceEdges admits the complete segment count against uint32 before assigning grid indices.
func (grid *surfaceGrid) insert(index int, segment surfaceSegment) error {
	tolerance := surfaceTolerance(segment.a, segment.b, grid.options.Tolerance)
	grid.tolerances[index] = tolerance

	low, high, cellErr := surfaceCellRange(segment, grid.cellSize, tolerance, grid.options.MaximumCandidates)
	if cellErr != nil {
		return cellErr
	}

	grid.minima[index] = low

	for x := low[0]; x <= high[0]; x++ {
		for y := low[1]; y <= high[1]; y++ {
			for z := low[2]; z <= high[2]; z++ {
				grid.work++
				if grid.work > grid.options.MaximumCandidates {
					return fmt.Errorf(
						"surface candidate work %d exceeds limit %d: %w",
						grid.work,
						grid.options.MaximumCandidates,
						ErrBounds,
					)
				}

				cell := surfaceCell{x, y, z}
				grid.cells[cell] = append(grid.cells[cell], uint32(index))
			}
		}
	}

	return nil
}

func (grid *surfaceGrid) connect(index, otherIndex uint32) error {
	segment, other := grid.segments[index], grid.segments[otherIndex]
	tolerance := grid.tolerances[index]
	firstConflicts := grid.contactConflicts(index, segment, other, tolerance)

	secondConflicts := grid.contactConflicts(otherIndex, other, segment, tolerance)
	if len(firstConflicts) > 0 || len(secondConflicts) > 0 {
		if grid.options.Ambiguity != SurfaceIsolateAmbiguity {
			return fmt.Errorf(
				"ambiguous surface contact receivers %d/%d along %v..%v; patch normals %v/%v; existing owners %v/%v: %w",
				grid.receivers[segment.owner].ID,
				grid.receivers[other.owner].ID,
				segment.a,
				segment.b,
				segment.normal,
				other.normal,
				grid.contactOwners(index),
				grid.contactOwners(otherIndex),
				ErrUnsupported,
			)
		}

		grid.isolate(segment, other, firstConflicts, secondConflicts)
	}

	if grid.isolated[segment.owner] || grid.isolated[other.owner] {
		return nil
	}

	grid.adjacency[segment.owner] = surfaceAddNeighbor(grid.adjacency[segment.owner], other.owner)
	grid.adjacency[other.owner] = surfaceAddNeighbor(grid.adjacency[other.owner], segment.owner)

	return nil
}

func (grid *surfaceGrid) contactOwners(index uint32) map[uint64]Vec3 {
	result := make(map[uint64]Vec3)

	if first := grid.firstContacts[index]; first.ownerPlusOne != 0 {
		receiver := grid.receivers[first.ownerPlusOne-1]
		result[receiver.ID] = receiver.GeometricNormal
	}

	for _, contact := range grid.contacts[index] {
		receiver := grid.receivers[contact.owner]
		result[receiver.ID] = receiver.GeometricNormal
	}

	return result
}

func surfaceFirstSharedCell(cell, first, second surfaceCell) bool {
	return cell == (surfaceCell{max(first[0], second[0]), max(first[1], second[1]), max(first[2], second[2])})
}

func surfaceAddNeighbor(neighbors []uint32, owner uint32) []uint32 {
	if slices.Contains(neighbors, owner) {
		return neighbors
	}

	return append(neighbors, owner)
}

// A positive-length collinear contact requires intersecting endpoint bounds.
// This broad phase uses the same world-precision tolerance as the exact test;
// it does not create adjacency or reject any exact positive contact.
func surfaceBoundsOverlap(first, second surfaceSegment, tolerance float32) bool {
	for axis := range 3 {
		if max(first.a[axis], first.b[axis])+tolerance < min(second.a[axis], second.b[axis]) ||
			max(second.a[axis], second.b[axis])+tolerance < min(first.a[axis], first.b[axis]) {
			return false
		}
	}

	return true
}

// Contact broad-phase cells are independent of reconstruction radius. Half-radius
// cells reduce unrelated candidate pairs while exact contact predicates and the
// breadth-first physical support radius remain unchanged. Preserve positive
// subnormal input radii rather than underflowing the broad-phase divisor to zero.
func surfaceContactCellSize(radius float32) float32 {
	const divisor = 2

	size := radius / divisor
	if size == 0 {
		return radius
	}

	return size
}
