package impostordata

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
)

func TestV4StorageAndUniformLayout(t *testing.T) {
	t.Parallel()

	for _, input := range []Document{fixtureDocument(), octahedralDocument(), layeredDocument()} {
		for _, storage := range []Storage{StorageRGBA8, StorageBC1, StorageETC2RGBA} {
			if input.Mode == ModeLayered && storage != StorageRGBA8 {
				continue
			}

			input.Storage = storage

			encoded, err := Encode(input)
			if err != nil {
				t.Fatal(err)
			}

			decoded, err := Decode(encoded)
			if err != nil || decoded.Mode != input.Mode || decoded.Storage != storage {
				t.Fatal("storage or capture mode changed", decoded, err)
			}

			// These offsets independently fix the current wire and WGSL ABI.
			if string(encoded[:8]) != "KIMPOST4" || binary.LittleEndian.Uint32(encoded[8:]) != 4 ||
				binary.LittleEndian.Uint32(encoded[100:]) != uint32(storage) || len(decoded.Uniform) != 3168 ||
				binary.LittleEndian.Uint32(decoded.Uniform[80:]) != math.Float32bits(float32(storage)) ||
				!bytes.Equal(decoded.Uniform[84:96], make([]byte, 12)) ||
				binary.LittleEndian.Uint32(decoded.Uniform[96:]) != math.Float32bits(input.Views[0].Right[0]) {
				t.Fatal("storage/header/view-table ABI mismatch")
			}

			for _, offset := range []int{80, 84, 88, 92, 96} {
				bad := decoded
				bad.Uniform = bytes.Clone(decoded.Uniform)
				bad.Uniform[offset] ^= 1

				if _, err := Encode(bad); err == nil {
					t.Fatal("admitted altered prepared storage or shifted view data", offset)
				}
			}
		}
	}
}

func TestV4StorageRejectsUnknownAndLossyLayeredDescriptors(t *testing.T) {
	t.Parallel()

	for _, input := range []Document{fixtureDocument(), octahedralDocument(), layeredDocument()} {
		input.Storage = StorageASTC6x6 + 1
		if _, err := Encode(input); err == nil {
			t.Fatal("admitted unknown storage")
		}
	}

	layered := layeredDocument()
	layered.Storage = StorageBC1

	if _, err := Encode(layered); err == nil {
		t.Fatal("admitted lossy storage for exact layered intervals/hierarchy")
	}

	layered.Storage = StorageETC2RGBA
	if _, err := Encode(layered); err == nil {
		t.Fatal("admitted ETC2 storage for exact layered intervals/hierarchy")
	}

	// This grid is legal RGBA8, but its base atlas is not block-aligned.
	raw := fixtureDocument()
	raw.Resolution, raw.CellSize, raw.Columns = 3, 5, 3
	raw.AtlasWidth, raw.AtlasHeight = 15, 10

	if _, err := Encode(raw); err != nil {
		t.Fatal("raw capture fixture rejected", err)
	}

	raw.Storage = StorageBC1
	if _, err := Encode(raw); err == nil {
		t.Fatal("admitted non-block-aligned BC1 atlas")
	}

	encoded, err := Encode(fixtureDocument())
	if err != nil {
		t.Fatal(err)
	}

	binary.LittleEndian.PutUint32(encoded[100:], math.MaxUint32)

	if _, err := Decode(encoded); err == nil {
		t.Fatal("admitted untrusted unknown storage word")
	}
}
