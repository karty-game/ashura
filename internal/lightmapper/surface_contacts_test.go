package lightmapper

import (
	"reflect"
	"testing"
)

func TestSurfaceContactSlotsRetainPartialIntervalsAndAllConflicts(t *testing.T) {
	t.Parallel()

	receivers := []Receiver{{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4}}
	segments := []surfaceSegment{
		{owner: 0, a: Vec3{0, 0, 0}, b: Vec3{4, 0, 0}},
		{owner: 1, a: Vec3{0, 0, 0}, b: Vec3{2, 0, 0}},
		{owner: 2, a: Vec3{2, 0, 0}, b: Vec3{4, 0, 0}},
		{owner: 3, a: Vec3{1, 0, 0}, b: Vec3{3, 0, 0}},
	}

	grid := newSurfaceGrid(receivers, segments, 1, SurfaceEdgeOptions{})
	for _, other := range []int{1, 2} {
		if conflicts := grid.contactConflicts(0, segments[0], segments[other], 0); len(conflicts) != 0 {
			t.Fatal("disjoint partial contacts became ambiguous", other, conflicts)
		}
	}

	conflicts := grid.contactConflicts(0, segments[0], segments[3], 0)
	if !reflect.DeepEqual(conflicts, []uint32{1, 2}) || len(grid.contactOwners(0)) != 3 {
		t.Fatal("dense first contact or sparse overflow lost an overlap/diagnostic", conflicts, grid.contactOwners(0))
	}
}

func TestParallelSurfaceContactsMatchIncrementalOracle(t *testing.T) {
	t.Parallel()
	scene, original := surfaceEdgeFixture(t)

	receivers := make([]Receiver, 0, (len(original)+1)*20)
	for group := range 20 {
		count := len(original)
		if group%2 == 0 {
			count++ // A duplicate footprint must retain the same ambiguity isolation.
		}

		for index := range count {
			receiver := original[index%len(original)]
			receiver.ID = uint64(len(receivers) + 1)
			receiver.Position[0] += float32(group) * 2

			receiver.Patches = append([]ReceiverPatch(nil), receiver.Patches...)
			for patch := range receiver.Patches {
				for corner := range 3 {
					receiver.Patches[patch].Positions[corner][0] += float32(group) * 2
				}
			}

			receivers = append(receivers, receiver)
		}
	}

	segments, err := surfaceSegmentsContext(t.Context(), scene, receivers)
	if err != nil {
		t.Fatal(err)
	}

	options := SurfaceEdgeOptions{MaximumCandidates: 1000000, MaximumEdges: 10000, Ambiguity: SurfaceIsolateAmbiguity}

	oracle := newSurfaceGrid(receivers, segments, 1, options)
	for index, segment := range segments {
		oracleSurfaceContactInsert(t, &oracle, uint32(index), segment)
	}

	oracle.pruneIsolated()

	for _, workers := range []int{1, 2, 7} {
		grid := newSurfaceGrid(receivers, segments, 1, options)
		for index, segment := range segments {
			if err := grid.insert(index, segment); err != nil {
				t.Fatal(err)
			}
		}

		if err := grid.connectContactsWithWorkers(t.Context(), workers); err != nil {
			t.Fatal(err)
		}

		grid.pruneIsolated()

		if grid.work != oracle.work || !reflect.DeepEqual(grid.adjacency, oracle.adjacency) ||
			!reflect.DeepEqual(grid.diagnostics, oracle.diagnostics) || !reflect.DeepEqual(grid.isolated, oracle.isolated) {
			t.Fatal("parallel contact order changed topology, ambiguity or candidate accounting", workers)
		}
	}
}

// This deliberately retains the original incremental index/compare/admit order,
// independently of the new immutable-index parallel candidate implementation.
func oracleSurfaceContactInsert(t *testing.T, grid *surfaceGrid, index uint32, segment surfaceSegment) {
	t.Helper()

	tolerance := surfaceTolerance(segment.a, segment.b, grid.options.Tolerance)

	low, high, err := surfaceCellRange(segment, grid.cellSize, tolerance, grid.options.MaximumCandidates)
	if err != nil {
		t.Fatal(err)
	}

	grid.tolerances[index], grid.minima[index] = tolerance, low
	for x := low[0]; x <= high[0]; x++ {
		for y := low[1]; y <= high[1]; y++ {
			for z := low[2]; z <= high[2]; z++ {
				grid.work++

				cell := surfaceCell{x, y, z}
				for _, other := range grid.cells[cell] {
					if grid.segments[other].owner == segment.owner || !surfaceFirstSharedCell(cell, low, grid.minima[other]) {
						continue
					}

					grid.work++
					if grid.contactEligible(index, other) {
						if err := grid.connect(index, other); err != nil {
							t.Fatal(err)
						}
					}
				}

				grid.cells[cell] = append(grid.cells[cell], index)
			}
		}
	}
}
