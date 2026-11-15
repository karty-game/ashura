package modeldata_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
	"reflect"
	"testing"

	"github.com/karty-game/karty-webgpu/internal/modeldata"
)

func TestOpaquePreparedBytesRetainV1(t *testing.T) {
	t.Parallel()

	encoded, err := modeldata.Encode(document())
	if err != nil {
		t.Fatal(err)
	}

	// Independent literal v1 record: header, padded fixture.glb/triangle,
	// seven unit float channels, three -1 slots, sidedness, geometry and indices.
	const opaqueV1SHA256 = "4f59e7319bf9c693fe94f32f913cdde5221ec1ca65275d4bcf0b4abbb2799a27"
	if fmt.Sprintf("%x", sha256.Sum256(encoded)) != opaqueV1SHA256 {
		t.Fatal("opaque v1 bytes changed")
	}
}

func TestMaskedPreparedV2RoundtripKeepsOpaquePartsAndOwnedViews(t *testing.T) {
	t.Parallel()

	source := document()
	source.Parts = append(source.Parts, source.Parts[0])
	source.Parts[1].Material.AlphaMask = true
	source.Parts[1].Material.AlphaCutoff = .5
	source.Parts[1].Material.Color[3] = .75

	encoded, err := modeldata.Encode(source)
	if err != nil {
		t.Fatal(err)
	}

	const wireV2 = 2
	if binary.LittleEndian.Uint32(encoded[8:]) != wireV2 {
		t.Fatal("masked document did not select v2")
	}

	decoded, err := modeldata.Decode(encoded)
	if err != nil || decoded.Source != source.Source || !reflect.DeepEqual(decoded.Parts, source.Parts) || len(decoded.Textures) != 0 {
		t.Fatal("v2 model semantics changed", err, decoded)
	}

	reencoded, err := modeldata.Encode(decoded)
	if err != nil || !bytes.Equal(encoded, reencoded) {
		t.Fatal("v2 model does not roundtrip exactly", err)
	}

	const indexBytes = 12

	last := decoded.Parts[len(decoded.Parts)-1]
	if &last.Indices[0] != &encoded[len(encoded)-indexBytes] {
		t.Fatal("v2 decoder rebuilt packed geometry")
	}
}

func TestMaskedPreparedV2RejectsMalformedCoverageAndTruncation(t *testing.T) {
	t.Parallel()

	source := document()
	source.Parts[0].Material.AlphaMask = true
	source.Parts[0].Material.AlphaCutoff = .5

	encoded, err := modeldata.Encode(source)
	if err != nil {
		t.Fatal(err)
	}

	const (
		wordBytes       = 4
		v2MaterialWords = 13
		maskWord        = 11
	)

	material := len(encoded) - len(source.Parts[0].Vertices) - len(source.Parts[0].Indices) - v2MaterialWords*wordBytes

	for _, test := range []struct {
		name   string
		mutate func([]byte) []byte
	}{
		{"unknown-flag", func(data []byte) []byte {
			binary.LittleEndian.PutUint32(data[material+maskWord*wordBytes:], 2)

			return data
		}},
		{"unused-cutoff", func(data []byte) []byte {
			binary.LittleEndian.PutUint32(data[material+maskWord*wordBytes:], 0)

			return data
		}},
		{"nan-cutoff", func(data []byte) []byte {
			binary.LittleEndian.PutUint32(data[material+(maskWord+1)*wordBytes:], math.Float32bits(float32(math.NaN())))

			return data
		}},
		{"negative-cutoff", func(data []byte) []byte {
			binary.LittleEndian.PutUint32(data[material+(maskWord+1)*wordBytes:], math.Float32bits(-1))

			return data
		}},
		{"high-cutoff", func(data []byte) []byte {
			binary.LittleEndian.PutUint32(data[material+(maskWord+1)*wordBytes:], math.Float32bits(2))

			return data
		}},
		{"downgrade", func(data []byte) []byte {
			binary.LittleEndian.PutUint32(data[8:], 1)

			return data
		}},
		{"truncated", func(data []byte) []byte {
			const missingCutoffSize = 108
			binary.LittleEndian.PutUint32(data[12:], missingCutoffSize)

			return data[:missingCutoffSize]
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if _, err := modeldata.Decode(test.mutate(bytes.Clone(encoded))); err == nil {
				t.Fatal("malformed v2 coverage admitted")
			}
		})
	}
}
