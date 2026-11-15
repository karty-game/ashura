// Package ktx2data admits prepared RGBA8, BC, ETC2 and ASTC KTX2 files without runtime dependencies.
// It performs no source-image decoding, decompression or transcoding.
package ktx2data

import (
	"encoding/binary"
	"errors"
	"math/bits"

	"github.com/karty-game/karty-webgpu/internal/texturedata"
)

const (
	identifier   = "\xabKTX 20\xbb\r\n\x1a\n"
	maxDimension = 4096

	headerSize     = 80
	levelEntrySize = 24
	maxFileBytes   = 96 << 20
	vkRGBA8        = 37
	vkRGBA8SRGB    = 43
	vkBC1          = 131
	vkBC1SRGB      = 132
	vkBC1RGBA      = 133
	vkBC1RGBASRGB  = 134
	vkBC6H         = 143
	vkBC7          = 145
	vkBC7SRGB      = 146
	vkETC2RGB      = 147
	vkETC2RGBSRGB  = 148
	vkETC2RGBA     = 151
	vkASTC4x4      = 157
)

var (
	// ErrFormat rejects unsupported container profiles.
	ErrFormat = errors.New("ktx2: expected prepared 2D RGBA8, BC, ETC2 or ASTC with no supercompression")
	// ErrBounds rejects malformed dimensions, metadata or mip ranges.
	ErrBounds = errors.New("ktx2: invalid header, level range or file size")
)

// Decode reads a prepared KTX2 container; returned mip slices reference encoded.
// Callers copy the payloads when taking ownership.
func Decode(encoded []byte) (texturedata.Data, error) {
	if len(encoded) < headerSize || len(encoded) > maxFileBytes || string(encoded[:12]) != identifier {
		return texturedata.Data{}, ErrBounds
	}

	format, width, height, levels, err := readHeader(encoded)
	if err != nil {
		return texturedata.Data{}, err
	}

	data := texturedata.Data{Width: int(width), Height: int(height), ColorSpace: texturedata.Linear,
		Format: payloadFormat(format), Levels: make([][]byte, levels)}
	if srgbFormat(format) {
		data.ColorSpace = texturedata.SRGB
	}

	indexEnd := uint64(headerSize) + uint64(levels)*levelEntrySize

	metadataEnd, err := metadataEnd(encoded, indexEnd, format)
	if err != nil {
		return texturedata.Data{}, err
	}

	ranges := make([][2]uint64, 0, levels)
	for level := range levels {
		entry := headerSize + int(level)*levelEntrySize
		offset := binary.LittleEndian.Uint64(encoded[entry:])
		length := binary.LittleEndian.Uint64(encoded[entry+8:])
		uncompressed := binary.LittleEndian.Uint64(encoded[entry+16:])

		row, rows := texturedata.Layout(data.Format, int(max(width>>level, 1)), int(max(height>>level, 1)))
		expected := uint64(row) * uint64(rows) //nolint:gosec // Header admission bounds both dimensions to 1..4096.

		alignment := levelAlignment(data.Format)

		if offset < metadataEnd || offset%alignment != 0 || offset > uint64(len(encoded)) || length > uint64(len(encoded))-offset ||
			length != expected ||
			uncompressed != length {
			return texturedata.Data{}, ErrBounds
		}

		for _, prior := range ranges {
			if offset < prior[1] && offset+length > prior[0] {
				return texturedata.Data{}, ErrBounds
			}
		}

		ranges = append(ranges, [2]uint64{offset, offset + length})
		data.Levels[level] = encoded[offset : offset+length]
	}

	return data, nil
}

func readHeader(encoded []byte) (format, width, height, levels uint32, err error) {
	read := func(offset int) uint32 { return binary.LittleEndian.Uint32(encoded[offset:]) }

	format, width, height, levels = read(12), read(20), read(24), read(40)
	if !supportedFormat(format) {
		return 0, 0, 0, 0, ErrFormat
	}

	if read(16) != 1 || read(28) != 0 || read(32) != 0 || read(36) != 1 || read(44) != 0 {
		return 0, 0, 0, 0, ErrFormat
	}

	if width == 0 || height == 0 || width > maxDimension || height > maxDimension || levels == 0 ||
		int(levels) > bits.Len32(max(width, height)) {
		return 0, 0, 0, 0, ErrBounds
	}

	if blockFormat(format) && (width%4 != 0 || height%4 != 0) {
		return 0, 0, 0, 0, ErrBounds
	}

	if headerSize+uint64(levels)*levelEntrySize > uint64(len(encoded)) {
		return 0, 0, 0, 0, ErrBounds
	}

	return format, width, height, levels, nil
}

func supportedFormat(format uint32) bool {
	switch format {
	case vkRGBA8, vkRGBA8SRGB, vkBC1, vkBC1SRGB, vkBC1RGBA, vkBC1RGBASRGB,
		vkBC6H, vkBC7, vkBC7SRGB, vkETC2RGB, vkETC2RGBSRGB, vkETC2RGBA, vkASTC4x4:
		return true
	default:
		return false
	}
}
