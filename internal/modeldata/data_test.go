package modeldata_test

import (
	"encoding/binary"
	"testing"

	"github.com/karty-game/karty-webgpu/internal/meshdata"
	"github.com/karty-game/karty-webgpu/internal/modeldata"
)

func document() modeldata.Document {
	return modeldata.Document{
		Source: "fixture.glb",
		Parts: []modeldata.Part{
			{
				Name:     "triangle",
				Vertices: make([]byte, 3*meshdata.VertexStride),
				Indices: []byte{
					0,
					0,
					0,
					0,
					1,
					0,
					0,
					0,
					2,
					0,
					0,
					0,
				},
				Material: modeldata.Material{
					Color:          [4]float32{1, 1, 1, 1},
					Roughness:      1,
					NormalStrength: 1,
					AOStrength:     1,
					Albedo:         -1,
					Normal:         -1,
					AO:             -1,
				},
			},
		},
	}
}

func TestPreparedContainerRoundTripAndCorruption(t *testing.T) {
	t.Parallel()

	encoded, err := modeldata.Encode(document())
	if err != nil {
		t.Fatal(err)
	}

	decoded, err := modeldata.Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}

	if decoded.Source != "fixture.glb" || len(decoded.Parts) != 1 || len(decoded.Parts[0].Vertices) != 96 {
		t.Fatalf("wrong container: %#v", decoded)
	}

	if &decoded.Parts[0].Vertices[0] != &encoded[len(encoded)-108] {
		t.Fatal("container decoding rebuilt geometry instead of returning immutable packed views")
	}

	for _, name := range []string{"version", "length", "count", "padding", "trailing"} {
		changed := append([]byte(nil), encoded...)

		switch name {
		case "version":
			binary.LittleEndian.PutUint32(changed[8:], 9)
		case "length":
			binary.LittleEndian.PutUint32(changed[12:], 1)
		case "count":
			binary.LittleEndian.PutUint32(changed[16:], modeldata.MaxParts+1)
		case "padding":
			changed[39] = 1
		case "trailing":
			changed = append(changed, 0)
		}

		if _, err := modeldata.Decode(changed); err == nil {
			t.Fatalf("accepted %s corruption", name)
		}
	}
}

func TestPreparedEncoderRejectsEmptyAndOutOfRangeMaterial(t *testing.T) {
	t.Parallel()

	if _, err := modeldata.Encode(modeldata.Document{}); err == nil {
		t.Fatal("encoded empty model")
	}

	invalid := document()

	invalid.Parts[0].Material.Albedo = 0
	if _, err := modeldata.Encode(invalid); err == nil {
		t.Fatal("encoded foreign texture reference")
	}
}
