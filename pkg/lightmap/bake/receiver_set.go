//go:build !js

package lightmapbake

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"math"
)

// ReceiverSet is the versioned build-only footprint and chart-owned gutter
// artifact. Its compact payload has no JSON pixels or embedded visibility scene.
type ReceiverSet struct {
	Schema        string
	Width, Height uint32
	Receivers     []Receiver
	Gutters       []uint32
}

const (
	receiverSetSchema  = "karty.lightmapper-receivers@2"
	receiverSetMaximum = 256 << 20
	receiverSetHeader  = 48
	receiverFixedBytes = 84
	receiverPatchBytes = 160
)

// EncodeReceiverSet preflights the complete compact size before allocating and
// preserves exact float32 bits, original barycentric pieces and world patches.
//
//nolint:gosec,mnd // G115: dimensions, counts and the complete byte size are validated before narrowing to the bounded wire/allocation types. Four-byte words and twelve-byte vectors/piece headers are the fixed KARTYRX2 wire layout.
func EncodeReceiverSet(set ReceiverSet) ([]byte, error) {
	set.Schema = receiverSetSchema
	if err := validReceiverSet(set); err != nil {
		return nil, err
	}

	size := uint64(16) + uint64(len(set.Gutters))*4
	for _, receiver := range set.Receivers {
		size += receiverFixedBytes + uint64(len(receiver.Patches))*receiverPatchBytes
		for _, piece := range receiver.Pieces {
			size += 12 + uint64(len(piece.Barycentric))*12
		}

		if size > receiverSetMaximum-receiverSetHeader {
			return nil, ErrBounds
		}
	}

	data := make([]byte, int(size)+receiverSetHeader)
	copy(data, "KARTYRX2")
	writer := receiverSetEncoder{data: data, offset: receiverSetHeader}
	writer.word(set.Width)
	writer.word(set.Height)
	writer.word(uint32(len(set.Receivers)))
	writer.word(uint32(len(set.Gutters)))

	for _, receiver := range set.Receivers {
		writer.receiver(receiver)
	}

	for _, owner := range set.Gutters {
		writer.word(owner)
	}

	if writer.offset != len(data) {
		return nil, ErrInvalid
	}

	binary.LittleEndian.PutUint64(data[8:], size)
	digest := sha256.Sum256(data[receiverSetHeader:])
	copy(data[16:48], digest[:])

	return data, nil
}

// DecodeReceiverSet validates the content digest before allocating arrays.
// Every count is checked against remaining bytes and a decoded-array budget.
//
//nolint:gosec // G115: signed lengths are nonnegative and bounded by the 256 MiB input admission before widening.
func DecodeReceiverSet(data []byte) (ReceiverSet, error) {
	var set ReceiverSet
	if len(data) < receiverSetHeader+16 || len(data) > receiverSetMaximum || string(data[:8]) != "KARTYRX2" ||
		binary.LittleEndian.Uint64(data[8:]) != uint64(len(data)-receiverSetHeader) {
		return set, ErrInvalid
	}

	digest := sha256.Sum256(data[receiverSetHeader:])
	if !bytes.Equal(digest[:], data[16:48]) {
		return set, ErrInvalid
	}

	decoder := receiverSetDecoder{data: data[receiverSetHeader:]}
	set.Schema = receiverSetSchema
	set.Width = decoder.word()
	set.Height = decoder.word()
	count, gutters := decoder.word(), decoder.word()

	if !receiverHeaderFits(set.Width, set.Height, count, gutters, decoder.remaining()) {
		return ReceiverSet{}, ErrInvalid
	}

	if !decoder.allocate(uint64(count), receiverAllocationBytes) || !decoder.allocate(uint64(gutters), 4) {
		return ReceiverSet{}, ErrBounds
	}

	set.Receivers = make([]Receiver, int(count))
	for i := range set.Receivers {
		if err := decoder.receiver(&set.Receivers[i]); err != nil {
			return ReceiverSet{}, err
		}
	}

	if uint64(gutters)*4 != uint64(decoder.remaining()) || decoder.failed {
		return ReceiverSet{}, ErrInvalid
	}

	set.Gutters = make([]uint32, int(gutters))
	for i := range set.Gutters {
		set.Gutters[i] = decoder.word()
	}

	return set, validReceiverSet(set)
}

// receiverSetEncoder writes into the exact allocation admitted above. Direct
// methods let scalar/vector writes inline instead of calling three closures for
// every field in large receiver artifacts.
type receiverSetEncoder struct {
	data   []byte
	offset int
}

func (writer *receiverSetEncoder) word(value uint32) {
	binary.LittleEndian.PutUint32(writer.data[writer.offset:], value)
	writer.offset += 4
}
func (writer *receiverSetEncoder) number(value float32) { writer.word(math.Float32bits(value)) }
func (writer *receiverSetEncoder) vector(value Vec3) {
	for _, component := range value {
		writer.number(component)
	}
}

//nolint:gosec // G115: all array lengths are preflighted against the 256 MiB payload bound; IDs and Side retain their exact wire bits.
func (writer *receiverSetEncoder) receiver(receiver Receiver) {
	writer.word(uint32(receiver.ID))
	writer.word(uint32(receiver.ID >> 32))
	writer.word(receiver.Pixel)
	writer.word(receiver.Chart)
	writer.word(uint32(receiver.Side))
	writer.number(receiver.Coverage)

	for _, value := range []Vec3{receiver.Position, receiver.GeometricNormal, receiver.Normal, receiver.Tangent} {
		writer.vector(value)
	}

	writer.number(receiver.Handedness)

	if receiver.Pieces == nil {
		writer.word(1 << 31)
	} else {
		writer.word(uint32(len(receiver.Pieces)))
	}

	writer.word(uint32(len(receiver.Patches)))

	for _, piece := range receiver.Pieces {
		writer.word(piece.Triangle)
		writer.number(piece.Area)
		writer.word(uint32(len(piece.Barycentric)))

		for _, bary := range piece.Barycentric {
			writer.vector(bary)
		}
	}

	for _, patch := range receiver.Patches {
		writer.word(patch.Triangle)
		writer.word(patch.Material)
		writer.number(patch.Area)

		for _, value := range patch.Positions {
			writer.vector(value)
		}

		for _, value := range patch.Normals {
			writer.vector(value)
		}

		for _, value := range patch.UV {
			writer.number(value[0])
			writer.number(value[1])
		}

		for _, value := range []Vec3{patch.GeometricNormal, patch.Tangent, patch.RawTangent, patch.RawBitangent} {
			writer.vector(value)
		}

		writer.number(patch.Handedness)
	}
}

//nolint:gosec // G115: remaining is a nonnegative decoder byte count.
func receiverHeaderFits(width, height, count, gutters uint32, remaining int) bool {
	pixels := uint64(width) * uint64(height)

	return width > 0 && height > 0 && width <= 2048 && height <= 2048 && count > 0 && uint64(count) <= pixels &&
		uint64(gutters) == pixels && uint64(count)*receiverFixedBytes+uint64(gutters)*4 <= uint64(remaining)
}
