//go:build !js

//nolint:mnd,gosec // Fixed Khronos KTX2 descriptors, admitted dimensions and native block offsets.
package assetbuild

import (
	"encoding/binary"
)

const (
	blockVKBC1          uint32 = 131
	blockVKBC1SRGB      uint32 = 132
	blockVKBC1RGBA      uint32 = 133
	blockVKBC1RGBASRGB  uint32 = 134
	blockVKBC6H         uint32 = 143
	blockVKBC7          uint32 = 145
	blockVKBC7SRGB      uint32 = 146
	blockVKETC2RGB      uint32 = 147
	blockVKETC2RGBSRGB  uint32 = 148
	blockVKETC2RGBA     uint32 = 151
	blockVKETC2RGBASRGB uint32 = 152
	blockVKASTC4x4      uint32 = 157
	blockVKASTC6x6      uint32 = 165
	blockVKASTC6x6SRGB  uint32 = 166
)

func writeNativeBlockKTX(format uint32, width, height int, levels [][]byte) []byte {
	return writeOrderedNativeBlockKTX(format, width, height, levels, false)
}

func writeCanonicalNativeBlockKTX(format uint32, width, height int, levels [][]byte) []byte {
	return writeOrderedNativeBlockKTX(format, width, height, levels, true)
}

func writeOrderedNativeBlockKTX(format uint32, width, height int, levels [][]byte, smallestFirst bool) []byte {
	descriptor := nativeBlockDescriptor(format)
	descriptorOffset := 80 + len(levels)*24
	alignment := int(descriptor[20])
	payloadOffset := (descriptorOffset + len(descriptor) + alignment - 1) &^ (alignment - 1)

	outputSize := payloadOffset
	for _, pixels := range levels {
		outputSize += len(pixels)
	}

	output := make([]byte, outputSize)
	copy(output, "\xabKTX 20\xbb\r\n\x1a\n")

	put := func(offset int, value uint32) { binary.LittleEndian.PutUint32(output[offset:], value) }
	put(12, format)
	put(16, 1)
	put(20, uint32(width))
	put(24, uint32(height))
	put(36, 1)
	put(40, uint32(len(levels)))
	put(48, uint32(descriptorOffset))
	put(52, uint32(len(descriptor)))
	copy(output[descriptorOffset:], descriptor)

	for index := range levels {
		level := index
		if smallestFirst {
			level = len(levels) - 1 - index
		}

		pixels := levels[level]
		entry := 80 + level*24
		binary.LittleEndian.PutUint64(output[entry:], uint64(payloadOffset))
		binary.LittleEndian.PutUint64(output[entry+8:], uint64(len(pixels)))
		binary.LittleEndian.PutUint64(output[entry+16:], uint64(len(pixels)))
		copy(output[payloadOffset:], pixels)
		payloadOffset += len(pixels)
	}

	return output
}

func nativeBlockDescriptor(format uint32) []byte {
	size, model, transfer, blockBytes, kind, upper := 44, byte(134), byte(1), byte(16), byte(0), uint32(0xffffffff)

	switch format {
	case blockVKBC1:
		model, blockBytes = 128, 8
	case blockVKBC1SRGB:
		model, transfer, blockBytes = 128, 2, 8
	case blockVKBC1RGBA:
		model, blockBytes, kind = 128, 8, 1
	case blockVKBC1RGBASRGB:
		model, transfer, blockBytes, kind = 128, 2, 8, 1
	case blockVKBC7SRGB:
		transfer = 2
	case blockVKBC6H:
		model, kind, upper = 133, 128, 0x3f800000
	case blockVKETC2RGB:
		model, blockBytes, kind = 161, 8, 2
	case blockVKETC2RGBSRGB:
		model, transfer, blockBytes, kind = 161, 2, 8, 2
	case blockVKETC2RGBA, blockVKETC2RGBASRGB:
		size, model, kind = 60, 161, 15

		if format == blockVKETC2RGBASRGB {
			transfer, kind = 2, 31 // Alpha is linear within the sRGB container.
		}
	case blockVKASTC4x4, blockVKASTC6x6, blockVKASTC6x6SRGB:
		model = 162

		if format == blockVKASTC6x6SRGB {
			transfer = 2
		}
	}

	descriptor := make([]byte, size)
	binary.LittleEndian.PutUint32(descriptor, uint32(size))
	binary.LittleEndian.PutUint16(descriptor[8:], 2)
	binary.LittleEndian.PutUint16(descriptor[10:], uint16(size-4))
	descriptor[12], descriptor[13], descriptor[14] = model, 1, transfer

	descriptor[16], descriptor[17], descriptor[20] = 3, 3, blockBytes
	if format == blockVKASTC6x6 || format == blockVKASTC6x6SRGB {
		descriptor[16], descriptor[17] = 5, 5
	}

	descriptor[30], descriptor[31] = blockBytes*8-1, kind
	binary.LittleEndian.PutUint32(descriptor[40:], upper)

	if format == blockVKETC2RGBA || format == blockVKETC2RGBASRGB {
		descriptor[30] = 63
		binary.LittleEndian.PutUint16(descriptor[44:], 64)
		descriptor[46], descriptor[47] = 63, 2
		binary.LittleEndian.PutUint32(descriptor[56:], 0xffffffff)
	}

	return descriptor
}
