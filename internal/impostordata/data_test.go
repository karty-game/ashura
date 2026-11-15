package impostordata

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
)

func fixtureDocument() Document {
	views := make([]View, 4)
	for index := range views {
		views[index] = View{Right: [3]float32{1, 0, 0}, Up: [3]float32{0, 1, 0}, Outward: [3]float32{0, 0, 1}}
	}

	return Document{
		Source:          "fixture.glb",
		Center:          [3]float32{0, 1, 0},
		Radius:          2,
		Roughness:       .8,
		Azimuths:        2,
		Elevations:      2,
		ElevationMin:    -.5,
		ElevationMax:    .5,
		AtlasWidth:      8,
		AtlasHeight:     8,
		Columns:         2,
		CellSize:        4,
		Resolution:      2,
		Padding:         1,
		MipLevels:       1,
		SourceTriangles: 12,
		Views:           views,
		Vertices:        make([]byte, quadVertexBytes),
		Indices:         make([]byte, quadIndexBytes),
		Color:           []byte{1},
		NormalDepth:     []byte{2},
	}
}

func TestPreparedEnvelopeRoundtripAndUniformAdmission(t *testing.T) {
	t.Parallel()

	input := fixtureDocument()

	data, err := Encode(input)
	if err != nil {
		t.Fatal(err)
	}

	decoded, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}

	if decoded.Source != input.Source || decoded.Center != input.Center || len(decoded.Views) != 4 || len(decoded.Uniform) != UniformSize {
		t.Fatalf("prepared descriptor lost metadata: %+v", decoded)
	}

	if &decoded.Vertices[0] != &data[len(data)-quadVertexBytes-quadIndexBytes-UniformSize-2] {
		t.Fatal("decoder copied/repacked prepared quad")
	}

	if len(input.Uniform) != 0 {
		t.Fatal("offline encoding mutated its caller document")
	}

	encodedAgain, err := Encode(decoded)
	if err != nil || !bytes.Equal(data, encodedAgain) {
		t.Fatal("prepared binary serialization is not deterministic")
	}
}

func TestMalformedPreparedDocumentsRejected(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"views", "radius", "basis", "layout-overflow", "layout", "mip", "uniform", "quad", "extra"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			document := fixtureDocument()

			switch name {
			case "views":
				document.Azimuths = MaxViews
			case "radius":
				document.Radius = float32(math.Inf(1))
			case "basis":
				document.Views[0].Up = document.Views[0].Right
			case "layout-overflow":
				document.Padding = 1<<31 + 1
			case "layout":
				document.AtlasWidth++
			case "mip":
				document.MipLevels = 3
			case "uniform":
				document.Uniform = bakeUniform(document)
				document.Uniform[0] ^= 1
			case "quad":
				document.Vertices = document.Vertices[:len(document.Vertices)-1]
			case "extra":
				data, err := Encode(document)
				if err != nil {
					t.Fatal(err)
				}

				if _, err := Decode(append(data, 0)); err == nil {
					t.Fatal("accepted trailing data")
				}

				return
			}

			if _, err := Encode(document); err == nil {
				t.Fatalf("accepted malformed %s", name)
			}
		})
	}
}

func TestTruncationAndUntrustedCounts(t *testing.T) {
	t.Parallel()

	data, err := Encode(fixtureDocument())
	if err != nil {
		t.Fatal(err)
	}

	for length := range data {
		if _, err := Decode(data[:length]); err == nil {
			t.Fatalf("accepted truncated length %d", length)
		}
	}

	for _, offset := range []int{12, 104} {
		mutated := bytes.Clone(data)
		binary.LittleEndian.PutUint32(mutated[offset:], math.MaxUint32)

		if _, err := Decode(mutated); err == nil {
			t.Fatalf("accepted unbounded count at %d", offset)
		}
	}
}
