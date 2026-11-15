package lightmapper

import "testing"

func surfaceEdgeFixture(t *testing.T) (*PreparedScene, []Receiver) {
	t.Helper()

	first := receiverFixtureTriangle()
	second := Triangle{ID: 2, Positions: [3]Vec3{{1, 0, 0}, {1, 1, 0}, {0, 1, 0}}, Normals: first.Normals, UV: first.UV}

	scene, err := PrepareScene(
		Scene{
			ID:            "surface",
			Space:         "world",
			MetresPerUnit: 1,
			Triangles:     []Triangle{first, second},
			Materials:     []Material{{Reflectance: Vec3{.5, .5, .5}}},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	result := make([]Receiver, 0, 2)

	for index, triangle := range []Triangle{first, second} {
		receivers, prepareErr := PrepareReceivers([]Triangle{triangle}, [][3]Vec2{triangle.UV}, 1, 1)
		if prepareErr != nil {
			t.Fatal(prepareErr)
		}

		if prepareErr = MaterializeReceiverPatches([]Triangle{triangle}, receivers); prepareErr != nil {
			t.Fatal(prepareErr)
		}

		receivers[0].ID = uint64(index + 11)
		receivers[0].Chart = uint32(index + 1)
		result = append(result, receivers[0])
	}

	return scene, result
}

func TestSurfaceEdgesJoinPhysicalSeamAcrossCharts(t *testing.T) {
	t.Parallel()
	scene, receivers := surfaceEdgeFixture(t)

	edges, err := BuildSurfaceEdges(scene, receivers, 1, SurfaceEdgeOptions{MaximumCandidates: 10000, MaximumEdges: 100})
	if err != nil || len(edges) != 2 || edges[0].From != 11 || edges[0].To != 12 || edges[1].From != 12 || edges[1].To != 11 {
		t.Fatal(edges, err)
	}
}

func TestSurfaceEdgesDoNotJoinUnconnectedNearbyGeometry(t *testing.T) {
	t.Parallel()

	scene, receivers := surfaceEdgeFixture(t)
	for index := range receivers[1].Patches {
		for corner := range 3 {
			receivers[1].Patches[index].Positions[corner][2] = .0001
		}
	}

	receivers[1].Position[2] = .0001

	edges, err := BuildSurfaceEdges(scene, receivers, 1, SurfaceEdgeOptions{MaximumCandidates: 10000, MaximumEdges: 100})
	if err != nil || len(edges) != 0 {
		t.Fatal(edges, err)
	}
}

func TestSurfaceEdgesFailAmbiguousOverlappingTopologyAndBounds(t *testing.T) {
	t.Parallel()
	scene, receivers := surfaceEdgeFixture(t)
	duplicate := receivers[0]
	duplicate.ID = 13

	receivers = append(receivers, duplicate)
	if _, err := BuildSurfaceEdges(scene, receivers, 1, SurfaceEdgeOptions{MaximumCandidates: 10000, MaximumEdges: 100}); err == nil {
		t.Fatal("accepted non-manifold duplicated surface")
	}

	if _, err := BuildSurfaceEdges(scene, receivers[:2], 1, SurfaceEdgeOptions{MaximumCandidates: 1, MaximumEdges: 100}); err == nil {
		t.Fatal("silently omitted candidates beyond declared bound")
	}
}

func TestSurfaceEdgesKeepCoincidentOppositeSidesIndependent(t *testing.T) {
	t.Parallel()

	scene, receivers := surfaceEdgeFixture(t)
	for _, front := range append([]Receiver(nil), receivers...) {
		back := front
		back.ID += 100
		back.Normal = Scale(back.Normal, -1)
		back.GeometricNormal = Scale(back.GeometricNormal, -1)

		back.Patches = append([]ReceiverPatch(nil), front.Patches...)
		for index := range back.Patches {
			back.Patches[index].GeometricNormal = Scale(back.Patches[index].GeometricNormal, -1)
			for corner := range 3 {
				back.Patches[index].Normals[corner] = Scale(back.Patches[index].Normals[corner], -1)
			}
		}

		receivers = append(receivers, back)
	}

	edges, err := BuildSurfaceEdges(scene, receivers, 1, SurfaceEdgeOptions{MaximumCandidates: 100000, MaximumEdges: 100})
	if err != nil || len(edges) != 4 {
		t.Fatal(edges, err)
	}

	for _, edge := range edges {
		if (edge.From < 100) != (edge.To < 100) {
			t.Fatal("crossed coincident fabric sides", edge)
		}
	}
}

func TestSurfaceEdgeToleranceFollowsFloatPrecisionAcrossWorldScales(t *testing.T) {
	t.Parallel()

	for _, scale := range []float32{1e-7, 1, 1e6} {
		scene, receivers := surfaceEdgeFixture(t)
		source := scene.Source()
		source.Triangles = append([]Triangle(nil), source.Triangles...)

		source.MetresPerUnit = 1 / scale
		for index := range source.Triangles {
			for corner := range 3 {
				source.Triangles[index].Positions[corner] = Scale(source.Triangles[index].Positions[corner], scale)
			}
		}

		prepared, err := PrepareScene(source)
		if err != nil {
			t.Fatal(err)
		}

		for index := range receivers {
			receivers[index].Position = Scale(receivers[index].Position, scale)
			for j := range receivers[index].Patches {
				for corner := range 3 {
					receivers[index].Patches[j].Positions[corner] = Scale(receivers[index].Patches[j].Positions[corner], scale)
				}
			}
		}

		edges, err := BuildSurfaceEdges(prepared, receivers, scale, SurfaceEdgeOptions{MaximumCandidates: 10000, MaximumEdges: 100})
		if err != nil || len(edges) != 2 {
			t.Fatal(scale, edges, err)
		}
	}
}

func TestSurfaceSupportRejectsIneligibleContactsBeforeTopology(t *testing.T) {
	t.Parallel()
	scene, receivers := surfaceEdgeFixture(t)
	receivers[1].Tangent = receivers[0].Tangent
	duplicate := receivers[0]
	duplicate.ID = 13
	duplicate.Normal = Scale(duplicate.Normal, -1)
	receivers = append(receivers, duplicate)

	options := SurfaceEdgeOptions{MaximumCandidates: 10000, MaximumEdges: 100}
	if _, err := BuildSurfaceEdges(scene, receivers, 1, options); err == nil {
		t.Fatal("strict unfiltered topology must expose the duplicate branch")
	}

	options.Support = &ReconstructionOptions{
		Radius:         1,
		PlaneTolerance: .001,
		NormalCos:      .95,
		TangentCos:     .95,
		NoiseScale:     1,
		Strength:       1,
		MaximumHalo:    100,
	}

	edges, err := BuildSurfaceEdges(scene, receivers, 1, options)
	if err != nil || len(edges) != 2 {
		t.Fatalf("ineligible guide contaminated admitted topology: %v %v", edges, err)
	}

	for _, edge := range edges {
		if edge.From == 13 || edge.To == 13 {
			t.Fatal("included zero-support receiver", edge)
		}
	}
}

func TestSurfaceIsolationRetainsAllConflictingOwnersAndInputSupport(t *testing.T) {
	t.Parallel()

	scene, receivers := surfaceEdgeFixture(t)
	for _, id := range []uint64{13, 14} {
		duplicate := receivers[0]
		duplicate.ID = id
		receivers = append(receivers, duplicate)
	}

	before, err := ReceiverHash(receivers)
	if err != nil {
		t.Fatal(err)
	}

	options := SurfaceEdgeOptions{MaximumCandidates: 10000, MaximumEdges: 100, Ambiguity: SurfaceIsolateAmbiguity}
	if _, err = BuildSurfaceEdges(scene, receivers, 1, options); err == nil {
		t.Fatal("isolation diagnostics could be silently discarded")
	}

	report, err := BuildSurfaceEdgesReport(scene, receivers, 1, options)
	if err != nil {
		t.Fatal(err)
	}

	if len(report.Edges) != 0 || len(report.IsolatedReceiverIDs) != 4 || len(report.AmbiguousContacts) == 0 {
		t.Fatalf("retained arbitrary earlier neighbor: %+v", report)
	}

	for index, id := range []uint64{11, 12, 13, 14} {
		if report.IsolatedReceiverIDs[index] != id {
			t.Fatal(report.IsolatedReceiverIDs)
		}
	}

	after, err := ReceiverHash(receivers)
	if err != nil || before != after {
		t.Fatalf("changed source coverage/patches: %s %s %v", before, after, err)
	}
}
