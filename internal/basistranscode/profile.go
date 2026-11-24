// Package basistranscode handles the explicitly approved ETC1S albedo loading
// profile. It produces compressed GPU blocks, never RGBA pixels.
//
//nolint:mnd // Fixed KTX2 field offsets, descriptor sizes and block dimensions.
package basistranscode

import (
	"context"
	"encoding/binary"
	"errors"
	"math/bits"

	"github.com/karty-game/ashura/internal/texturedata"
)

var ErrProfile = errors.New("basis: expected bounded opaque sRGB ETC1S albedo with complete mip chain")
var ErrUnsupported = errors.New("basis: ETC1S loading requires the web loader and a BC or ETC2 capable device")

// Decoder owns a reusable asynchronous transcoder for one loader lifetime.
type Decoder interface {
	Transcode(ctx context.Context, encoded []byte, format texturedata.Format) (texturedata.Data, error)
	Close()
}

type Info struct{ Width, Height, Levels int }

// Inspect admits header, descriptor, codebook and complete nonoverlapping mip
// ranges before foreign-code processing. The transcoder also validates syntax.
func Inspect(data []byte) (Info, error) {
	if len(data) < 80 || len(data) > 16<<20 || string(data[:12]) != "\xabKTX 20\xbb\r\n\x1a\n" {
		return Info{}, ErrProfile
	}

	u32 := func(at int) uint32 { return binary.LittleEndian.Uint32(data[at:]) }

	width, height, levels := u32(20), u32(24), u32(40)
	if !validHeader(data, width, height, levels) {
		return Info{}, ErrProfile
	}

	end := uint64(80 + levels*24)
	if end > uint64(len(data)) {
		return Info{}, ErrProfile
	}

	rangeOK := func(offset, length uint64) bool {
		return offset >= end && offset <= uint64(len(data)) && length <= uint64(len(data))-offset
	}

	dfd, size := uint64(u32(48)), uint64(u32(52))
	if !rangeOK(dfd, size) || dfd%4 != 0 || size != 44 {
		return Info{}, ErrProfile
	}

	descriptor := data[dfd : dfd+size]
	if !validDescriptor(descriptor) {
		return Info{}, ErrProfile
	}

	end = dfd + size

	keyEnd, err := keyMetadataEnd(data, end)
	if err != nil {
		return Info{}, err
	}

	end = keyEnd

	sgd, sgdSize := binary.LittleEndian.Uint64(data[64:]), binary.LittleEndian.Uint64(data[72:])
	if !rangeOK(sgd, sgdSize) || sgd%8 != 0 || sgdSize < 20+uint64(levels)*20 {
		return Info{}, ErrProfile
	}

	codebook := data[sgd : sgd+sgdSize]
	if !validCodebook(codebook, levels) {
		return Info{}, ErrProfile
	}

	end = sgd + sgdSize

	if err := validLevelRanges(data, codebook, end, levels); err != nil {
		return Info{}, err
	}

	return Info{int(width), int(height), int(levels)}, nil
}

func validCodebook(data []byte, levels uint32) bool {
	if binary.LittleEndian.Uint16(data) == 0 || binary.LittleEndian.Uint16(data) > 16128 || binary.LittleEndian.Uint16(data[2:]) == 0 ||
		binary.LittleEndian.Uint16(data[2:]) > 16128 {
		return false
	}

	total := uint64(20 + levels*20)
	for at := 4; at < 20; at += 4 {
		total += uint64(binary.LittleEndian.Uint32(data[at:]))
	}

	return total == uint64(len(data))
}

func keyMetadataEnd(data []byte, end uint64) (uint64, error) {
	key, size := uint64(binary.LittleEndian.Uint32(data[56:])), uint64(binary.LittleEndian.Uint32(data[60:]))
	if size == 0 {
		if key != 0 {
			return 0, ErrProfile
		}

		return end, nil
	}

	if key < end || key%4 != 0 || key > uint64(len(data)) || size > uint64(len(data))-key {
		return 0, ErrProfile
	}

	return key + size, nil
}

func validHeader(data []byte, width, height, levels uint32) bool {
	u32 := func(at int) uint32 { return binary.LittleEndian.Uint32(data[at:]) }

	return u32(12) == 0 && u32(16) == 1 && width >= 4 && height >= 4 && width <= 1024 && height <= 1024 && width&(width-1) == 0 &&
		height&(height-1) == 0 &&
		int(levels) == bits.Len32(max(width, height)) &&
		u32(28) == 0 &&
		u32(32) == 0 &&
		u32(36) == 1 &&
		u32(44) == 1
}
func validDescriptor(descriptor []byte) bool {
	// One opaque ETC1S RGB sample, sRGB transfer and 4x4 blocks.
	return binary.LittleEndian.Uint32(descriptor) == 44 && binary.LittleEndian.Uint32(descriptor[4:]) == 0 &&
		binary.LittleEndian.Uint16(descriptor[8:]) == 2 &&
		binary.LittleEndian.Uint16(descriptor[10:]) == 40 &&
		descriptor[12] == 163 &&
		descriptor[13] == 1 &&
		descriptor[14] == 2 &&
		descriptor[15] == 0 &&
		descriptor[16] == 3 &&
		descriptor[17] == 3 &&
		descriptor[18] == 0 &&
		descriptor[19] == 0 &&
		descriptor[31] == 0
}

func validLevelRanges(data, codebook []byte, end uint64, levels uint32) error {
	rangeOK := func(offset, length uint64) bool {
		return offset >= end && offset <= uint64(len(data)) && length <= uint64(len(data))-offset
	}

	var ranges [][2]uint64

	for level := range levels {
		entry := 80 + level*24

		offset, length := binary.LittleEndian.Uint64(data[entry:]), binary.LittleEndian.Uint64(data[entry+8:])
		if !rangeOK(offset, length) || length == 0 || binary.LittleEndian.Uint64(data[entry+16:]) != 0 {
			return ErrProfile
		}

		for _, prior := range ranges {
			if offset < prior[1] && offset+length > prior[0] {
				return ErrProfile
			}
		}

		ranges = append(ranges, [2]uint64{offset, offset + length})

		desc := codebook[20+level*20 : 40+level*20]
		if binary.LittleEndian.Uint32(desc) != 0 || binary.LittleEndian.Uint32(desc[4:]) != 0 ||
			uint64(binary.LittleEndian.Uint32(desc[8:])) != length ||
			binary.LittleEndian.Uint32(desc[12:]) != 0 ||
			binary.LittleEndian.Uint32(desc[16:]) != 0 {
			return ErrProfile
		}
	}

	return nil
}
