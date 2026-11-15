package ktx2data

import "github.com/karty-game/karty-webgpu/internal/texturedata"

func payloadFormat(format uint32) texturedata.Format {
	switch format {
	case vkBC1, vkBC1SRGB, vkBC1RGBA, vkBC1RGBASRGB:
		return texturedata.BC1
	case vkBC6H:
		return texturedata.BC6H
	case vkBC7, vkBC7SRGB:
		return texturedata.BC7
	case vkETC2RGB, vkETC2RGBSRGB:
		return texturedata.ETC2RGB
	case vkETC2RGBA:
		return texturedata.ETC2RGBA
	case vkASTC4x4:
		return texturedata.ASTC4x4
	default:
		return texturedata.RGBA8
	}
}

func blockFormat(format uint32) bool { return payloadFormat(format) != texturedata.RGBA8 }

func srgbFormat(format uint32) bool {
	return format == vkRGBA8SRGB || format == vkBC1SRGB || format == vkBC1RGBASRGB || format == vkBC7SRGB || format == vkETC2RGBSRGB
}

// BC1Alpha identifies the punchthrough profile of a container already admitted
// by Decode. It reads header metadata only, without repeating mip admission.
func BC1Alpha(encoded []byte) bool {
	if len(encoded) < headerSize || len(encoded) > maxFileBytes || string(encoded[:12]) != identifier {
		return false
	}

	//nolint:dogsled // Only the container format is needed; header admission still checks dimensions and level count.
	format, _, _, _, err := readHeader(encoded)

	return err == nil && (format == vkBC1RGBA || format == vkBC1RGBASRGB)
}

//nolint:mnd // Native Khronos DFD sizes include the fixed header and one or two 16-byte samples.
func descriptorLength(format uint32) uint64 {
	if format == vkETC2RGBA {
		return 60
	}

	if blockFormat(format) {
		return 44
	}

	return descriptorSize
}

//nolint:mnd // KTX2 native mip offsets align to their fixed 4-, 8- or 16-byte texel/block size.
func levelAlignment(format texturedata.Format) uint64 {
	switch format {
	case texturedata.RGBA8:
		return 4
	case texturedata.BC1, texturedata.ETC2RGB:
		return 8
	case texturedata.BC6H, texturedata.BC7, texturedata.ETC2RGBA, texturedata.ASTC4x4:
		return 16
	default:
		return 0
	}
}
