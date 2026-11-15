//go:build !js

package assetbuild

import (
	"encoding/binary"
	"image"
	"slices"
)

//nolint:gosec,mnd // Fixed KTX2/DFD byte layout; all casts use bounded source dimensions, channels and mip counts.
func encodeKTX(levels []*image.NRGBA, srgb bool) []byte {
	const dfdLength = 92

	dfdOffset := 80 + len(levels)*24
	data := make([]byte, dfdOffset+dfdLength)
	copy(data, []byte{0xab, 0x4b, 0x54, 0x58, 0x20, 0x32, 0x30, 0xbb, 0x0d, 0x0a, 0x1a, 0x0a})

	put := func(offset int, value uint32) { binary.LittleEndian.PutUint32(data[offset:], value) }

	format := uint32(37)
	if srgb {
		format = 43
	}

	put(12, format)
	put(16, 1)
	put(20, uint32(levels[0].Bounds().Dx()))
	put(24, uint32(levels[0].Bounds().Dy()))
	put(36, 1)
	put(40, uint32(len(levels)))
	put(48, uint32(dfdOffset))
	put(52, dfdLength)
	put(dfdOffset, dfdLength)
	binary.LittleEndian.PutUint16(data[dfdOffset+8:], 2)
	binary.LittleEndian.PutUint16(data[dfdOffset+10:], 88)
	data[dfdOffset+12] = 1
	data[dfdOffset+14] = 1

	data[dfdOffset+20] = 4
	if srgb {
		data[dfdOffset+13] = 1
		data[dfdOffset+14] = 2
	}

	for channel := range 4 {
		offset := dfdOffset + 28 + channel*16
		binary.LittleEndian.PutUint16(data[offset:], uint16(channel*8))
		data[offset+2] = 7

		data[offset+3] = byte(channel)
		if channel == 3 {
			data[offset+3] = 31
		}

		binary.LittleEndian.PutUint32(data[offset+12:], 255)
	}

	for index, level := range slices.Backward(levels) {
		entry := 80 + index*24
		payload := level.Pix

		binary.LittleEndian.PutUint64(data[entry:], uint64(len(data)))
		binary.LittleEndian.PutUint64(data[entry+8:], uint64(len(payload)))
		binary.LittleEndian.PutUint64(data[entry+16:], uint64(len(payload)))
		data = append(data, payload...) //nolint:makezero // The serialized header/index/DFD prefix precedes mip payload bytes.
	}

	return data
}
