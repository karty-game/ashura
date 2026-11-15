package lightmapper

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"math"
	"testing"
)

// The independent scalar serializer pins the established identity byte order,
// including flush boundaries and nil slices, while production batches writes.
func TestReceiverHashKeepsScalarWire(t *testing.T) {
	t.Parallel()

	receivers := make([]Receiver, 37)
	for index := range receivers {
		receivers[index] = testPointReceiver()
		receivers[index].ID = uint64(index) + 1<<40
		receivers[index].Patches = append([]ReceiverPatch(nil), receivers[index].Patches...)
		receivers[index].Normal[0] = math.Float32frombits(1 << 31)

		receivers[index].Pieces = []FootprintPiece{{Triangle: 19, Barycentric: []Vec3{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}}, Area: .5}}
		if index%2 == 0 {
			receivers[index].Pieces = nil
		}

		if index%3 == 0 {
			receivers[index].Patches = []ReceiverPatch{}
		}
	}

	digest := sha256.New()
	_, _ = digest.Write([]byte(receiverIdentityVersion))
	write := func(value any) {
		if err := binary.Write(digest, binary.LittleEndian, value); err != nil {
			t.Fatal(err)
		}
	}
	length := func(count int, absent bool) {
		if absent {
			write(uint64(math.MaxUint64))
		} else {
			//nolint:gosec // Counts come directly from nonnegative slice lengths.
			write(uint64(count))
		}
	}
	length(len(receivers), false)

	for _, receiver := range receivers {
		write(receiver.ID)
		write(receiver.Pixel)
		write(receiver.Chart)
		write(receiver.Side)
		length(len(receiver.Pieces), receiver.Pieces == nil)

		for _, piece := range receiver.Pieces {
			write(piece.Triangle)
			length(len(piece.Barycentric), piece.Barycentric == nil)
			write(piece.Barycentric)
			write(piece.Area)
		}

		length(len(receiver.Patches), receiver.Patches == nil)

		for _, patch := range receiver.Patches {
			for _, value := range []any{patch.Positions, patch.Normals, patch.UV, patch.Area, patch.Triangle, patch.Material,
				patch.GeometricNormal, patch.Tangent, patch.RawTangent, patch.RawBitangent, patch.Handedness} {
				write(value)
			}
		}

		for _, value := range []any{receiver.Coverage, receiver.Position, receiver.GeometricNormal, receiver.Normal, receiver.Tangent, receiver.Handedness} {
			write(value)
		}
	}

	got, err := ReceiverHash(receivers)
	if err != nil || got != hex.EncodeToString(digest.Sum(nil)) {
		t.Fatal("batched identity changed scalar wire", got, err)
	}
}
