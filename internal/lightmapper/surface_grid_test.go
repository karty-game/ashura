package lightmapper

import (
	"slices"
	"testing"
)

// Compare the exact admitted pair order, not only set membership. Every later
// geometry/contact predicate therefore sees the same input and visit order as
// the old global tested-pair map, including collinear and T-shaped cell spans.
func TestFirstSharedCellMatchesPairMapOracle(t *testing.T) {
	t.Parallel()

	type span struct{ low, high surfaceCell }

	type pair struct {
		first, second int
		cell          surfaceCell
	}

	spans := []span{
		{surfaceCell{-4, 0, 0}, surfaceCell{4, 0, 0}},
		{surfaceCell{-2, 0, 0}, surfaceCell{2, 0, 0}},
		{surfaceCell{0, -3, 0}, surfaceCell{0, 3, 0}},
		{surfaceCell{0, 0, -2}, surfaceCell{0, 0, 2}},
		{surfaceCell{2, -1, -1}, surfaceCell{3, 1, 1}},
		{surfaceCell{10, 10, 10}, surfaceCell{12, 11, 10}},
		{surfaceCell{11, 10, 10}, surfaceCell{13, 11, 11}},
		{surfaceCell{0, 0, 0}, surfaceCell{0, 0, 0}},
	}
	cells := make(map[surfaceCell][]int)
	tested := make(map[[2]int]bool)

	var oracle, canonical []pair

	for current, bounds := range spans {
		for x := bounds.low[0]; x <= bounds.high[0]; x++ {
			for y := bounds.low[1]; y <= bounds.high[1]; y++ {
				for z := bounds.low[2]; z <= bounds.high[2]; z++ {
					cell := surfaceCell{x, y, z}
					for _, other := range cells[cell] {
						key := [2]int{other, current}
						if !tested[key] {
							tested[key] = true

							oracle = append(oracle, pair{other, current, cell})
						}

						if surfaceFirstSharedCell(cell, bounds.low, spans[other].low) {
							canonical = append(canonical, pair{other, current, cell})
						}
					}

					cells[cell] = append(cells[cell], current)
				}
			}
		}
	}

	if len(oracle) == 0 || !slices.Equal(oracle, canonical) {
		t.Fatalf("changed candidate support/order: oracle=%v canonical=%v", oracle, canonical)
	}
}

func TestHalfRadiusCellsRetainLongDiagonalPhysicalContact(t *testing.T) {
	t.Parallel()

	positions := [][3]Vec3{
		{{0, 0, 0}, {8, 8, 0}, {4, 4.01, 0}},
		{{0, 0, 0}, {4.01, 4, 0}, {8, 8, 0}},
	}

	triangles := make([]Triangle, 0, len(positions))
	for index, corners := range positions {
		for corner, point := range corners {
			corners[corner] = Vec3{point[0] + .25*point[1], point[1] + .5*point[0], .125*point[0] + .25*point[1]}
		}

		normal := Normalize(Cross(Sub(corners[1], corners[0]), Sub(corners[2], corners[0])))
		triangles = append(
			triangles,
			Triangle{
				ID:        uint64(index + 1),
				Positions: corners,
				Normals:   [3]Vec3{normal, normal, normal},
				UV:        [3]Vec2{{0, 0}, {1, 0}, {0, 1}},
			},
		)
	}

	scene, err := PrepareScene(
		Scene{
			ID:            "diagonal-contact",
			Space:         "world",
			MetresPerUnit: 1,
			Triangles:     triangles,
			Materials:     []Material{{Reflectance: Vec3{.5, .5, .5}}},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	receivers := make([]Receiver, 0, len(triangles))
	for index, triangle := range triangles {
		prepared, prepareErr := PrepareReceivers([]Triangle{triangle}, [][3]Vec2{triangle.UV}, 1, 1)
		if prepareErr != nil {
			t.Fatal(prepareErr)
		}

		if prepareErr = MaterializeReceiverPatches([]Triangle{triangle}, prepared); prepareErr != nil {
			t.Fatal(prepareErr)
		}

		prepared[0].ID = uint64(index + 1)
		prepared[0].Chart = uint32(index)
		receivers = append(receivers, prepared[0])
	}

	edges, err := BuildSurfaceEdges(scene, receivers, 1, SurfaceEdgeOptions{MaximumCandidates: 100000, MaximumEdges: 10})
	if err != nil || len(edges) != 2 {
		t.Fatal("lost long diagonal collinear contact across many broad-phase cells", edges, err)
	}

	if _, err = BuildSurfaceEdges(scene, receivers, 1, SurfaceEdgeOptions{MaximumCandidates: 1, MaximumEdges: 10}); err == nil {
		t.Fatal("diagonal span bypassed complete candidate bounds")
	}
}
