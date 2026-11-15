package lightmapkernel_test

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/karty-game/karty-webgpu/internal/lightmapdata"
	"github.com/karty-game/karty-webgpu/internal/lightmapfixture"
	"github.com/karty-game/karty-webgpu/internal/lightmapkernel"
)

func admittedRNM(t *testing.T) *lightmapdata.Document {
	t.Helper()

	encoded, err := lightmapdata.Encode(lightmapfixture.RNMDocument())
	if err != nil {
		t.Fatal(err)
	}

	document, err := lightmapdata.Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}

	return document
}

func TestRNMParameterPackingAndBounds(t *testing.T) {
	t.Parallel()
	document := admittedRNM(t)
	scene := lightmapkernel.RNMScene{
		Points:        []lightmapkernel.Point{{Position: [3]float32{1, 2, 3}, Color: [3]float32{1, .5, .25}, Intensity: 2, Range: 10}},
		Samples:       16,
		Bounces:       1,
		Seed:          42,
		RowsPerRecord: 8,
	}

	parameters, points, scale, normalized, err := lightmapkernel.RNMParameters(document, scene)
	if err != nil {
		t.Fatal(err)
	}

	if len(points) != lightmapkernel.RNMMaxPoints*lightmapkernel.RNMPointStride || normalized.RowsPerRecord != 1 || scale <= 2 {
		t.Fatal("point buffer, row bound or bounce scale invalid")
	}

	words := func(offset int) uint32 { return binary.LittleEndian.Uint32(parameters[offset:]) }
	if words(0) != 4 || words(4) != 1 || words(16) != 0 || words(20) != 8 || words(40) != 1 ||
		words(44) != 16 ||
		words(48) != 1 ||
		words(52) != 42 {
		t.Fatal("RNM uniform ABI changed")
	}

	if math.Float32frombits(binary.LittleEndian.Uint32(points[16:])) != 2 ||
		math.Float32frombits(binary.LittleEndian.Uint32(points[28:])) != .015 {
		t.Fatal("energy or default bias packed incorrectly")
	}

	lightmapkernel.RNMRows(&parameters, 7, 3)

	if words(56) != 7 || words(60) != 3 || words(52) != 42 {
		t.Fatal("tile interval changed deterministic sampling seed")
	}

	for _, entry := range []string{"bake_rnm", "resolve_rnm"} {
		desc := lightmapkernel.RNMDescription(document, entry)
		if len(desc.Layout) != 7 || desc.Layout[3].MinSize != 4*48 || desc.EntryPoint != entry {
			t.Fatal("portable RNM pipeline layout changed")
		}
	}
}

func TestRNMRejectsUnpreparedAndInvalidScenes(t *testing.T) {
	t.Parallel()
	document := admittedRNM(t)
	point := lightmapkernel.Point{Color: [3]float32{1, 1, 1}, Intensity: 1, Range: 10}

	cases := []lightmapkernel.RNMScene{
		{Samples: 65}, {Bounces: 2}, {SkyColor: [3]float32{-1, 0, 0}}, {GroundColor: [3]float32{float32(math.NaN()), 0, 0}},
		{Points: make([]lightmapkernel.Point, 4097)}, {Points: []lightmapkernel.Point{{Range: 10, Intensity: float32(math.Inf(1))}}},
	}
	for _, scene := range cases {
		if _, _, _, _, err := lightmapkernel.RNMParameters(document, scene); err == nil {
			t.Fatal("invalid RNM setting admitted")
		}
	}

	scalar := lightmapfixture.Document()
	if _, _, _, _, err := lightmapkernel.RNMParameters(
		&scalar,
		lightmapkernel.RNMScene{Points: []lightmapkernel.Point{point}},
	); err == nil {
		t.Fatal("unprepared v1 document admitted to RNM")
	}
	// Settings overflow must be rejected before finite values reach GPU arithmetic.
	point.Intensity = math.MaxFloat32
	if _, _, _, _, err := lightmapkernel.RNMParameters(
		document,
		lightmapkernel.RNMScene{Points: []lightmapkernel.Point{point, point}},
	); err == nil {
		t.Fatal("overflowing total energy admitted")
	}
}
