//go:build !js

package lightmapbake

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
)

func TestReceiverSetWriterPreservesScalarBytes(t *testing.T) {
	t.Parallel()
	set := compactReceiverFixture(t, 32)
	set.Receivers[0].Normal[0] = math.Float32frombits(1 << 31)

	for _, piecesAbsent := range []bool{false, true} {
		if piecesAbsent {
			set.Receivers[0].Pieces = nil
		}

		got, err := EncodeReceiverSet(set)
		if err != nil {
			t.Fatal(err)
		}

		var payload []byte

		put := func(value uint32) { payload = binary.LittleEndian.AppendUint32(payload, value) }
		number := func(value float32) { put(math.Float32bits(value)) }
		vector := func(value Vec3) {
			for _, channel := range value {
				number(channel)
			}
		}

		put(set.Width)
		put(set.Height)
		//nolint:gosec // The fixture is bounded to 32 by 32 pixels.
		put(uint32(len(set.Receivers)))
		//nolint:gosec // The fixture is bounded to 32 by 32 pixels.
		put(uint32(len(set.Gutters)))

		for _, receiver := range set.Receivers {
			scalarEncodeReceiver(receiver, put, number, vector)
		}

		for _, owner := range set.Gutters {
			put(owner)
		}

		if !bytes.Equal(got[receiverSetHeader:], payload) {
			t.Fatal("direct writer changed scalar receiver wire")
		}
	}
}

//nolint:gosec // G115: all array lengths are preflighted against the 256 MiB payload bound; IDs and Side retain their exact wire bits.
func scalarEncodeReceiver(receiver Receiver, put func(uint32), number func(float32), vector func(Vec3)) {
	put(uint32(receiver.ID))
	put(uint32(receiver.ID >> 32))
	put(receiver.Pixel)
	put(receiver.Chart)
	put(uint32(receiver.Side))
	number(receiver.Coverage)

	for _, value := range []Vec3{receiver.Position, receiver.GeometricNormal, receiver.Normal, receiver.Tangent} {
		vector(value)
	}

	number(receiver.Handedness)

	if receiver.Pieces == nil {
		put(1 << 31)
	} else {
		put(uint32(len(receiver.Pieces)))
	}

	put(uint32(len(receiver.Patches)))

	for _, piece := range receiver.Pieces {
		put(piece.Triangle)
		number(piece.Area)
		put(uint32(len(piece.Barycentric)))

		for _, bary := range piece.Barycentric {
			vector(bary)
		}
	}

	for _, patch := range receiver.Patches {
		put(patch.Triangle)
		put(patch.Material)
		number(patch.Area)

		for _, value := range patch.Positions {
			vector(value)
		}

		for _, value := range patch.Normals {
			vector(value)
		}

		for _, value := range patch.UV {
			number(value[0])
			number(value[1])
		}

		for _, value := range []Vec3{patch.GeometricNormal, patch.Tangent, patch.RawTangent, patch.RawBitangent} {
			vector(value)
		}

		number(patch.Handedness)
	}
}
