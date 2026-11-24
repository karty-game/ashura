package modeldata_test

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"

	"github.com/karty-game/ashura/internal/meshdata"
	"github.com/karty-game/ashura/internal/modeldata"
)

func TestPreparedBoundsPreservePayloadAndRequireEnclosure(t *testing.T) {
	t.Parallel()

	source := document()
	binary.LittleEndian.PutUint32(source.Parts[0].Vertices, math.Float32bits(-2))

	data, err := modeldata.EncodeWithBounds(source)
	if err != nil {
		t.Fatal(err)
	}

	decoded, err := modeldata.Decode(data)
	if err != nil {
		t.Fatal(err)
	}

	part := decoded.Parts[0]
	if !part.Bounds.Valid || part.Bounds.Minimum[0] != -2 ||
		!bytes.Equal(part.Vertices, source.Parts[0].Vertices) || !bytes.Equal(part.Indices, source.Parts[0].Indices) {
		t.Fatal("V3 lost the part bounds or changed its upload bytes", part.Bounds)
	}

	roundTrip, err := modeldata.Encode(decoded)
	if err != nil || !bytes.Equal(data, roundTrip) {
		t.Fatal("V3 did not round-trip", err)
	}

	if _, err := meshdata.FromOwnedWithBounds(part.Vertices, part.Indices, part.Bounds); err != nil {
		t.Fatal("valid offline bounds were rejected", err)
	}

	part.Bounds.Minimum[0] = -1
	if _, err := meshdata.FromOwnedWithBounds(part.Vertices, part.Indices, part.Bounds); err == nil {
		t.Fatal("admitted an offline bound that excluded a vertex")
	}
}
