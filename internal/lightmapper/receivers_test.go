package lightmapper

import (
	"math"
	"testing"
)

func receiverFixtureTriangle() Triangle {
	return Triangle{
		ID:        1,
		Positions: [3]Vec3{{0, 0, 0}, {1, 0, 0}, {0, 1, 0}},
		Normals:   [3]Vec3{{0, 0, 1}, {0, 0, 1}, {0, 0, 1}},
		UV:        [3]Vec2{{0, 0}, {1, 0}, {0, 1}},
	}
}

func TestFootprintsRetainSliversAndArea(t *testing.T) {
	t.Parallel()

	triangle := receiverFixtureTriangle()
	coordinates := [][3]Vec2{{{.02, .02}, {.98, .02}, {.98, .021}}}

	receivers, err := PrepareReceivers([]Triangle{triangle}, coordinates, 8, 8)
	if err != nil {
		t.Fatal(err)
	}

	if len(receivers) < 7 {
		t.Fatalf("thin triangle lost surface coverage: %d receivers", len(receivers))
	}

	var area float64
	for _, receiver := range receivers {
		area += float64(receiver.Coverage)
		if receiver.Coverage <= 0 {
			t.Fatal(receiver)
		}
	}

	expected := float64(coordinates[0][1][0]-coordinates[0][0][0]) * float64(coordinates[0][2][1]-coordinates[0][0][1]) * 32
	if math.Abs(area-expected) > 1e-6 {
		t.Fatalf("area %g want %g", area, expected)
	}
}

func TestCoveredUVSampleIsInsideSurface(t *testing.T) {
	t.Parallel()

	triangle := receiverFixtureTriangle()

	scene, err := PrepareScene(
		Scene{
			ID:            "receiver",
			Space:         "world",
			MetresPerUnit: 1,
			Triangles:     []Triangle{triangle},
			Materials:     []Material{{Reflectance: Vec3{.5, .5, .5}}},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	receivers, err := PrepareReceivers([]Triangle{triangle}, [][3]Vec2{triangle.UV}, 4, 4)
	if err != nil {
		t.Fatal(err)
	}

	var total float32
	for _, receiver := range receivers {
		total += receiver.Coverage

		for sample := range 20 {
			a, b, c := float32(sample)/20, float32((sample*7)%20)/20, float32((sample*13)%20)/20

			point, sampleErr := SampleReceiver(scene, receiver, a, b, c)
			if sampleErr != nil {
				t.Fatal(sampleErr)
			}

			x, y := point.Position[0], point.Position[1]
			if x < 0 || y < 0 || x+y > 1.00001 || point.PDF != 1/receiver.Coverage || point.Normal[2] < .999 {
				t.Fatal(point)
			}
		}
	}

	if total != 8 {
		t.Fatal(total)
	}
}

func TestIndependentWorldPatchesDoNotUseOccluderTriangleIndices(t *testing.T) {
	t.Parallel()

	triangle := receiverFixtureTriangle()

	receivers, err := PrepareReceivers([]Triangle{triangle}, [][3]Vec2{triangle.UV}, 1, 1)
	if err != nil {
		t.Fatal(err)
	}

	patches, err := receiverPatches([]Triangle{triangle}, receivers[0])
	if err != nil {
		t.Fatal(err)
	}

	receivers[0].Patches = patches

	result, err := ReceiverPatches(nil, receivers[0])
	if err != nil || len(result) == 0 {
		t.Fatal(result, err)
	}
}

func TestFootprintsRejectPositiveUVOverlapBelowFullCoverage(t *testing.T) {
	t.Parallel()

	triangle := receiverFixtureTriangle()
	other := triangle
	other.ID = 2

	coordinates := [][3]Vec2{{{.1, .1}, {.4, .1}, {.1, .4}}, {{.2, .1}, {.5, .1}, {.2, .4}}}
	if _, err := PrepareReceivers([]Triangle{triangle, other}, coordinates, 1, 1); err == nil {
		t.Fatal("overlap accepted because summed coverage was below one")
	}
}

func TestFootprintsAcceptOppositeWindingsAtExactSharedUVBoundary(t *testing.T) {
	t.Parallel()

	first := receiverFixtureTriangle()
	second := first
	second.ID = 2
	second.Positions = [3]Vec3{{1, 0, 0}, {1, 1, 0}, {0, 1, 0}}

	coordinates := [][3]Vec2{{{0, 0}, {1, 0}, {0, 1}}, {{1, 1}, {1, 0}, {0, 1}}}
	for _, size := range []uint32{1, 17, 128} {
		if _, err := PrepareReceivers([]Triangle{first, second}, coordinates, size, size); err != nil {
			t.Fatal(size, err)
		}
	}
}

func TestPreciseClippedFansRetainQuantizedSliver(t *testing.T) {
	t.Parallel()

	triangle := receiverFixtureTriangle()
	a, c := float64(.066439845), float64(3.36781e-8)
	polygon := []receiverClipVertex{
		{uv: [2]float64{0, 0}, bary: [3]float64{a, 1 - a - c, c}},
		{uv: [2]float64{1e-7, 0}, bary: [3]float64{a + c, 1 - a - c, 0}},
		{uv: [2]float64{0, 6.4235876e-7}, bary: [3]float64{a, 1 - a, 0}},
	}

	piece := receiverPixelPiece{
		polygon: polygon,
		piece:   FootprintPiece{Triangle: 0, Area: float32(receiverPolygonArea(polygon)), Barycentric: make([]Vec3, 3)},
	}
	for index, vertex := range polygon {
		for axis := range 3 {
			piece.piece.Barycentric[index][axis] = float32(vertex.bary[axis])
		}
	}

	if receiverBaryArea(piece.piece.Barycentric[0], piece.piece.Barycentric[1], piece.piece.Barycentric[2]) != 0 {
		t.Fatal("fixture must collapse in the former fixed barycentric projection")
	}

	patches, err := receiverClippedPatches(triangle, piece)
	if err != nil || len(patches) != 1 || patches[0].Area != piece.piece.Area {
		t.Fatalf("lost positive clipped measure: %v %+v", err, patches)
	}

	receiver := Receiver{Side: 1, Coverage: piece.piece.Area, Pieces: []FootprintPiece{piece.piece}, Patches: patches}
	if err = receiverGuide([]Triangle{triangle}, &receiver); err != nil {
		t.Fatal(err)
	}

	if receiver.Coverage <= 0 || receiver.GeometricNormal == (Vec3{}) {
		t.Fatalf("invalid sliver guide: %+v", receiver)
	}
}
