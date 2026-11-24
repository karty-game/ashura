package lightmapdata_test

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"

	"github.com/karty-game/ashura/internal/lightmapdata"
	"github.com/karty-game/ashura/internal/lightmapfixture"
)

func TestRNMRoundTripUsesContiguousOwnedGPUData(t *testing.T) {
	t.Parallel()

	document := lightmapfixture.RNMDocument()

	encoded, err := lightmapdata.Encode(document)
	if err != nil {
		t.Fatal(err)
	}

	if binary.LittleEndian.Uint32(encoded[8:]) != 2 {
		t.Fatal("RNM extension did not select version 2")
	}

	loaded, err := lightmapdata.Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}

	if !loaded.RNM() {
		t.Fatal("lost directional extension")
	}

	offset := 0
	for _, field := range [][]byte{loaded.Nodes, loaded.Triangles, loaded.Receivers, loaded.Padding, loaded.Frames, loaded.Surfaces} {
		if !bytes.Equal(field, loaded.GPUData[offset:offset+len(field)]) || &field[0] != &loaded.GPUData[offset] {
			t.Fatal("prepared field does not alias contiguous GPU payload")
		}

		offset += len(field)
	}

	if offset != len(loaded.GPUData) {
		t.Fatal("unexpected GPU payload bytes")
	}

	clear(encoded)

	if math.Float32frombits(binary.LittleEndian.Uint32(loaded.Frames)) != 1 ||
		math.Float32frombits(binary.LittleEndian.Uint32(loaded.Surfaces)) != .5 {
		t.Fatal("decode retained mutable caller bytes")
	}
}

func TestMalformedRNMInputsRejected(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"missing-frames", "missing-surfaces", "frame-length", "surface-length", "tangent-zero", "tangent-nan", "tangent-not-orthogonal", "handedness", "uncovered-frame", "reflectance-negative", "reflectance-energy", "surface-nan", "surface-sidedness"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			document := lightmapfixture.RNMDocument()
			put := func(data []byte, offset int, value float32) {
				binary.LittleEndian.PutUint32(data[offset:], math.Float32bits(value))
			}

			switch name {
			case "missing-frames":
				document.Frames = nil
			case "missing-surfaces":
				document.Surfaces = nil
			case "frame-length":
				document.Frames = document.Frames[:16]
			case "surface-length":
				document.Surfaces = append(document.Surfaces, make([]byte, 16)...)
			case "tangent-zero":
				clear(document.Frames[:12])
			case "tangent-nan":
				put(document.Frames, 0, float32(math.NaN()))
			case "tangent-not-orthogonal":
				put(document.Frames, 0, 0)
				put(document.Frames, 4, 1)
			case "handedness":
				put(document.Frames, 12, 0)
			case "uncovered-frame":
				put(document.Frames, 32, 1)
			case "reflectance-negative":
				put(document.Surfaces, 0, -.1)
			case "reflectance-energy":
				put(document.Surfaces, 0, 1)
			case "surface-nan":
				put(document.Surfaces, 0, float32(math.NaN()))
			case "surface-sidedness":
				put(document.Surfaces, 12, 2)
			}

			if _, err := lightmapdata.Encode(document); err == nil {
				t.Fatal("accepted invalid RNM extension")
			}
		})
	}
}

func TestRNMExtensionLengthHeaderCoveredByChecksum(t *testing.T) {
	t.Parallel()

	encoded, err := lightmapdata.Encode(lightmapfixture.RNMDocument())
	if err != nil {
		t.Fatal(err)
	}

	for _, offset := range []int{8, 104, 108} {
		corrupt := append([]byte(nil), encoded...)

		corrupt[offset] ^= 1
		if _, err := lightmapdata.Decode(corrupt); err == nil {
			t.Fatalf("accepted mutated RNM header %d", offset)
		}
	}
}
