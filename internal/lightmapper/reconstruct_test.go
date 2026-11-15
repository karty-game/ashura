package lightmapper

import "testing"

func TestReconstructionUsesFinalizedImmutableCrossTileHalo(t *testing.T) {
	t.Parallel()

	records := []ReconstructionSample{
		{
			ID:         1,
			Space:      "world",
			Position:   Vec3{0, 0, 0},
			Normal:     Vec3{0, 0, 1},
			Tangent:    Vec3{1, 0, 0},
			Handedness: 1,
			Side:       1,
			Samples:    16,
			Finalized:  true,
		},
		{
			ID:         2,
			Space:      "world",
			Position:   Vec3{.1, 0, 0},
			Normal:     Vec3{0, 0, 1},
			Tangent:    Vec3{1, 0, 0},
			Handedness: 1,
			Side:       1,
			Samples:    16,
			Finalized:  true,
		},
	}
	for i := range records {
		for kernel := range 4 {
			for channel := range 3 {
				records[i].Mean[kernel][channel] = float32(i + 1)
				records[i].Variance[kernel][channel] = 10
			}
		}
	}

	edges := []ReconstructionEdge{{From: 1, To: 2, Weight: 1, Visible: true}, {From: 2, To: 1, Weight: 1, Visible: true}}
	options := ReconstructionOptions{
		Radius:         1,
		PlaneTolerance: .01,
		NormalCos:      .99,
		TangentCos:     .99,
		NoiseScale:     4,
		Strength:       1,
		MaximumHalo:    2,
	}

	together, err := ReconstructLinear([]uint64{1, 2}, records, edges, options)
	if err != nil {
		t.Fatal(err)
	}

	first, err := ReconstructLinear([]uint64{1}, records, edges, options)
	if err != nil {
		t.Fatal(err)
	}

	second, err := ReconstructLinear([]uint64{2}, records, edges, options)
	if err != nil {
		t.Fatal(err)
	}

	if together[0] != first[0] || together[1] != second[0] || together[0][0][0] <= 1 || together[1][0][0] >= 2 {
		t.Fatal(together, first, second)
	}

	if records[0].Mean[0][0] != 1 || records[1].Mean[0][0] != 2 {
		t.Fatal("mutated prior pass")
	}

	records[1].Finalized = false
	if _, err = ReconstructLinear([]uint64{1}, records, edges, options); err == nil {
		t.Fatal("accepted unfinished cross-tile halo")
	}
}

func TestReconstructionRejectsUnrelatedSpaceAndOccludedEdges(t *testing.T) {
	t.Parallel()

	records := []ReconstructionSample{
		{ID: 1, Space: "first", Normal: Vec3{0, 0, 1}, Tangent: Vec3{1, 0, 0}, Handedness: 1, Side: 1, Samples: 16, Finalized: true},
		{ID: 2, Space: "second", Normal: Vec3{0, 0, 1}, Tangent: Vec3{1, 0, 0}, Handedness: 1, Side: 1, Samples: 16, Finalized: true},
	}
	records[1].Mean[0] = Vec3{10, 10, 10}
	records[0].Variance[0] = Vec3{10, 10, 10}
	records[1].Variance[0] = Vec3{10, 10, 10}
	options := ReconstructionOptions{
		Radius:         1,
		PlaneTolerance: .01,
		NormalCos:      .99,
		TangentCos:     .99,
		NoiseScale:     4,
		Strength:       1,
		MaximumHalo:    2,
	}

	result, err := ReconstructLinear([]uint64{1}, records, []ReconstructionEdge{{From: 1, To: 2, Weight: 1, Visible: true}}, options)
	if err != nil || result[0] != records[0].Mean {
		t.Fatal(result, err)
	}

	records[1].Space = "first"

	result, err = ReconstructLinear([]uint64{1}, records, []ReconstructionEdge{{From: 1, To: 2, Weight: 1, Visible: false}}, options)
	if err != nil || result[0] != records[0].Mean {
		t.Fatal(result, err)
	}
}
