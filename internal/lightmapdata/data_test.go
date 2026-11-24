package lightmapdata_test

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/karty-game/ashura/internal/lightmapdata"
	"github.com/karty-game/ashura/internal/lightmapfixture"
)

func TestRoundTripOwnsValidatedBytes(t *testing.T) {
	t.Parallel()

	document := lightmapfixture.Document()

	encoded, err := lightmapdata.Encode(document)
	if err != nil {
		t.Fatal(err)
	}

	loaded, err := lightmapdata.Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}

	if loaded.Width != 4 || loaded.Height != 1 || loaded.ModelSHA256 != document.ModelSHA256 {
		t.Fatal("lost metadata")
	}

	clear(encoded)

	if loaded.ModelSHA256[0] != 1 || binary.LittleEndian.Uint32(loaded.Nodes[28:]) != 1 {
		t.Fatal("retained caller's mutable bytes")
	}
}

func TestMalformedInputsRejected(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"cycle", "triangle-range", "normal", "padding-range", "padding-uncovered", "receiver-size", "uv-range", "nan"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			document := lightmapfixture.Document()

			switch name {
			case "cycle":
				binary.LittleEndian.PutUint32(document.Nodes[28:], 0)
			case "triangle-range":
				binary.LittleEndian.PutUint32(document.Nodes[28:], 2)
			case "normal":
				clear(document.Receivers[16:28])
			case "padding-range":
				binary.LittleEndian.PutUint32(document.Padding, 10)
			case "padding-uncovered":
				binary.LittleEndian.PutUint32(document.Padding[8:], 2)
			case "receiver-size":
				document.Receivers = document.Receivers[:32]
			case "uv-range":
				binary.LittleEndian.PutUint32(document.UVs[0], math.Float32bits(2))
			case "nan":
				binary.LittleEndian.PutUint32(document.Triangles, math.Float32bits(float32(math.NaN())))
			}

			if _, err := lightmapdata.Encode(document); err == nil {
				t.Fatal("accepted invalid bake input")
			}
		})
	}
}

func TestTruncationAndPayloadCorruptionRejected(t *testing.T) {
	t.Parallel()

	encoded, err := lightmapdata.Encode(lightmapfixture.Document())
	if err != nil {
		t.Fatal(err)
	}

	for i := range encoded {
		if _, err = lightmapdata.Decode(encoded[:i]); err == nil {
			t.Fatalf("accepted truncation %d", i)
		}
	}

	encoded[len(encoded)-1] ^= 1
	if _, err = lightmapdata.Decode(encoded); err == nil {
		t.Fatal("accepted checksum mismatch")
	}
}

func TestHeaderIntegrityIncludesModelBinding(t *testing.T) {
	t.Parallel()

	encoded, err := lightmapdata.Encode(lightmapfixture.Document())
	if err != nil {
		t.Fatal(err)
	}

	for _, offset := range []int{12, 16, 20, 24, 28, 32, 36, 40, 71} {
		corrupt := append([]byte(nil), encoded...)

		corrupt[offset] ^= 1
		if _, err = lightmapdata.Decode(corrupt); err == nil {
			t.Fatalf("accepted mutated header byte %d", offset)
		}
	}
}
