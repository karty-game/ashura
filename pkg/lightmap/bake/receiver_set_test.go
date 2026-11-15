//go:build !js

package lightmapbake

import (
	"crypto/sha256"
	"encoding/binary"
	"math"
	"reflect"
	"testing"
)

func compactReceiverFixture(t *testing.T, size uint32) ReceiverSet {
	t.Helper()

	triangle := Triangle{
		ID:        1,
		Positions: [3]Vec3{{0, 0, 0}, {1, 0, 0}, {0, 1, 0}},
		Normals:   [3]Vec3{{0, 0, 1}, {0, 0, 1}, {0, 0, 1}},
		UV:        [3]Vec2{{0, 0}, {1, 0}, {0, 1}},
	}

	receivers, err := PrepareReceivers(Scene{Triangles: []Triangle{triangle}}, [][3]Vec2{triangle.UV}, size, size)
	if err != nil {
		t.Fatal(err)
	}

	set := ReceiverSet{Schema: receiverSetSchema, Width: size, Height: size, Receivers: receivers, Gutters: make([]uint32, int(size*size))}
	for i := range set.Gutters {
		set.Gutters[i] = math.MaxUint32
	}

	for _, receiver := range receivers {
		set.Gutters[receiver.Pixel] = receiver.Pixel
	}

	return set
}

func TestCompactReceiverSetRoundtripPreservesAllSupportAndFrames(t *testing.T) {
	t.Parallel()
	set := compactReceiverFixture(t, 16)
	set.Receivers[0].ID = 0xfedcba9876543210
	set.Receivers[0].Patches[0].RawTangent = Vec3{3, 4, 5}
	set.Receivers[0].Patches[0].RawBitangent = Vec3{-2, 6, 7}

	data, err := EncodeReceiverSet(set)
	if err != nil {
		t.Fatal(err)
	}

	if string(data[:8]) != "KARTYRX2" || len(data) > len(set.Receivers)*1024+len(set.Gutters)*4+64 {
		t.Fatal("not a compact typed receiver artifact", len(data))
	}

	decoded, err := DecodeReceiverSet(data)
	if err != nil || !reflect.DeepEqual(set, decoded) {
		t.Fatal("footprints/frame/identity changed", err)
	}
	// Independent world-patch callers may have no barycentric source. Preserve
	// nil versus empty for exact receiver-identity round trips.
	set.Receivers[0].Pieces = nil

	data, err = EncodeReceiverSet(set)
	if err != nil {
		t.Fatal(err)
	}

	decoded, err = DecodeReceiverSet(data)
	if err != nil || !reflect.DeepEqual(set, decoded) {
		t.Fatal("nil source identity changed", err)
	}
}

func TestCompactReceiverSetRejectsCorruptionAndImpossibleCounts(t *testing.T) {
	t.Parallel()

	encoded, err := EncodeReceiverSet(compactReceiverFixture(t, 4))
	if err != nil {
		t.Fatal(err)
	}

	corrupted := append([]byte(nil), encoded...)

	corrupted[len(corrupted)-1] ^= 1
	if _, err = DecodeReceiverSet(corrupted); err == nil {
		t.Fatal("accepted payload corruption")
	}

	if _, err = DecodeReceiverSet(encoded[:len(encoded)-1]); err == nil {
		t.Fatal("accepted truncation")
	}

	corrupted = append([]byte(nil), encoded...)
	binary.LittleEndian.PutUint32(corrupted[receiverSetHeader+8:], math.MaxUint32)
	digest := sha256.Sum256(corrupted[receiverSetHeader:])
	copy(corrupted[16:48], digest[:])

	if _, err = DecodeReceiverSet(corrupted); err == nil {
		t.Fatal("allocated from impossible receiver count")
	}

	corrupted = append([]byte(nil), encoded...)
	// Receiver counts occupy the final eight bytes of its fixed record.
	binary.LittleEndian.PutUint32(corrupted[receiverSetHeader+16+76:], math.MaxUint32)
	digest = sha256.Sum256(corrupted[receiverSetHeader:])
	copy(corrupted[16:48], digest[:])

	if _, err = DecodeReceiverSet(corrupted); err == nil {
		t.Fatal("accepted impossible piece allocation")
	}
}
