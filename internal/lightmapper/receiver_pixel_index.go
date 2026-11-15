package lightmapper

import (
	"encoding/binary"
	"math"
)

type receiverPixelIndex struct {
	dense  []uint32
	sparse map[uint32]uint32
	base   uint32
}

// Dense scratch stays bounded by the row domain's share of the caller's
// triangle/UV data. Very large sparse admitted domains retain a sparse index;
// their dimensions alone must not force a new page-sized allocation.
//
//nolint:gosec // The caller supplies a slice length; both fixed numeric record types have strictly positive binary sizes. Dense cells are capped before int conversion.
func newReceiverPixelIndex(triangles int, width, height, first, last uint32) receiverPixelIndex {
	rows := uint64(last - first)
	cells := uint64(width) * rows
	inputBytes := uint64(triangles) * uint64(binary.Size(Triangle{})+binary.Size([3]Vec2{}))
	rowBytes := inputBytes/uint64(height)*rows + inputBytes%uint64(height)*rows/uint64(height)

	const indexBytes = 4
	if cells*indexBytes <= rowBytes && cells <= uint64(math.MaxInt)/indexBytes {
		return receiverPixelIndex{dense: make([]uint32, int(cells)), base: first * width}
	}

	return receiverPixelIndex{sparse: make(map[uint32]uint32)}
}

func (index receiverPixelIndex) get(pixel uint32) uint32 {
	if index.dense != nil {
		return index.dense[pixel-index.base]
	}

	return index.sparse[pixel]
}

func (index receiverPixelIndex) set(pixel, slot uint32) {
	if index.dense != nil {
		index.dense[pixel-index.base] = slot

		return
	}

	index.sparse[pixel] = slot
}
