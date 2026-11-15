//go:build !js

package lightmapbake

import (
	"encoding/binary"
	"math"
	"unsafe"

	"github.com/karty-game/karty-webgpu/internal/lightmapper"
)

type ReceiverPatch = lightmapper.ReceiverPatch

// These compile-time sizes account for actual host-array allocation separately
// from the architecture-independent wire layout. No unsafe pointer is accessed.
const (
	receiverAllocationBytes = uint64(unsafe.Sizeof(Receiver{}))
	pieceAllocationBytes    = uint64(unsafe.Sizeof(FootprintPiece{}))
	patchAllocationBytes    = uint64(unsafe.Sizeof(ReceiverPatch{}))
)

type receiverSetDecoder struct {
	data      []byte
	offset    int
	allocated uint64
	failed    bool
}

func (decoder *receiverSetDecoder) remaining() int { return len(decoder.data) - decoder.offset }
func (decoder *receiverSetDecoder) word() uint32 {
	if decoder.failed || decoder.remaining() < 4 {
		decoder.failed = true

		return 0
	}

	value := binary.LittleEndian.Uint32(decoder.data[decoder.offset:])
	decoder.offset += 4

	return value
}
func (decoder *receiverSetDecoder) number() float32 { return math.Float32frombits(decoder.word()) }
func (decoder *receiverSetDecoder) vector() Vec3 {
	return Vec3{decoder.number(), decoder.number(), decoder.number()}
}
func (decoder *receiverSetDecoder) allocate(count, size uint64) bool {
	if count > receiverSetMaximum/size || decoder.allocated > receiverSetMaximum-count*size {
		return false
	}

	decoder.allocated += count * size

	return true
}
func receiverSetFinite(value float32) bool {
	return !math.IsNaN(float64(value)) && !math.IsInf(float64(value), 0)
}
func receiverSetPositive(value float32) bool { return receiverSetFinite(value) && value > 0 }
func receiverSetVector(value Vec3) bool {
	return receiverSetFinite(value[0]) && receiverSetFinite(value[1]) && receiverSetFinite(value[2])
}

//nolint:gosec,mnd // G115: counts are bounded against remaining wire bytes and the decoded-array allocation budget before conversion. The KARTYRX2 wire uses two 32-bit ID halves and bit31 for nil piece-slice provenance.
func (decoder *receiverSetDecoder) receiver(receiver *Receiver) error {
	low, high := decoder.word(), decoder.word()
	receiver.ID = uint64(low) | uint64(high)<<32
	receiver.Pixel = decoder.word()
	receiver.Chart = decoder.word()
	receiver.Side = int32(decoder.word())
	receiver.Coverage = decoder.number()
	receiver.Position = decoder.vector()
	receiver.GeometricNormal = decoder.vector()
	receiver.Normal = decoder.vector()
	receiver.Tangent = decoder.vector()
	receiver.Handedness = decoder.number()
	pieceWord, patches := decoder.word(), decoder.word()
	nilPieces := pieceWord == 1<<31

	pieces := pieceWord &^ (uint32(1) << 31)
	if pieceWord&(1<<31) != 0 && !nilPieces {
		return ErrInvalid
	}

	if uint64(pieces)*48+uint64(patches)*receiverPatchBytes > uint64(decoder.remaining()) {
		return ErrInvalid
	}

	if !decoder.allocate(uint64(pieces), pieceAllocationBytes) || !decoder.allocate(uint64(patches), patchAllocationBytes) {
		return ErrBounds
	}

	if !nilPieces {
		receiver.Pieces = make([]FootprintPiece, int(pieces))
	}

	receiver.Patches = make([]ReceiverPatch, int(patches))
	for j := range receiver.Pieces {
		if err := decoder.piece(&receiver.Pieces[j]); err != nil {
			return err
		}
	}

	for j := range receiver.Patches {
		decoder.patch(&receiver.Patches[j])
	}

	return nil
}

//nolint:gosec // G115: remaining bytes are a nonnegative bounded decoder length before conversion.
func (decoder *receiverSetDecoder) piece(piece *FootprintPiece) error {
	piece.Triangle = decoder.word()
	piece.Area = decoder.number()

	vertices := decoder.word()
	if vertices < 3 || vertices > 16 || uint64(vertices)*12 > uint64(decoder.remaining()) {
		return ErrInvalid
	}

	if !decoder.allocate(uint64(vertices), 12) {
		return ErrBounds
	}

	piece.Barycentric = make([]Vec3, int(vertices))
	for vertex := range piece.Barycentric {
		piece.Barycentric[vertex] = decoder.vector()
	}

	return nil
}

func (decoder *receiverSetDecoder) patch(patch *ReceiverPatch) {
	patch.Triangle = decoder.word()
	patch.Material = decoder.word()

	patch.Area = decoder.number()
	for vertex := range patch.Positions {
		patch.Positions[vertex] = decoder.vector()
	}

	for vertex := range patch.Normals {
		patch.Normals[vertex] = decoder.vector()
	}

	for vertex := range patch.UV {
		patch.UV[vertex] = Vec2{decoder.number(), decoder.number()}
	}

	patch.GeometricNormal = decoder.vector()
	patch.Tangent = decoder.vector()
	patch.RawTangent = decoder.vector()
	patch.RawBitangent = decoder.vector()
	patch.Handedness = decoder.number()
}
