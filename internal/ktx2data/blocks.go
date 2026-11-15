package ktx2data

import "encoding/binary"

// Native containers are prepared offline. Admission inspects metadata and block
// ranges only; decoding stays in the GPU's texture sampler.
func validBlockDescriptor(data []byte, format uint32) bool {
	length := descriptorLength(format)
	if uint64(len(data)) != length || uint64(binary.LittleEndian.Uint32(data)) != length ||
		binary.LittleEndian.Uint32(data[4:]) != 0 || binary.LittleEndian.Uint16(data[8:]) != 2 ||
		uint64(binary.LittleEndian.Uint16(data[10:])) != length-4 {
		return false
	}

	model, plane, bits, channel, upper := blockDescriptorProfile(format)

	transfer := byte(1)
	if srgbFormat(format) {
		transfer = 2
	}

	if !validBlockLayout(data, model, plane, transfer) {
		return false
	}

	if format == vkETC2RGBA {
		return validBlockSample(data[28:44], 0, bits, 15, upper) &&
			validBlockSample(data[44:60], 64, bits, channel, upper)
	}

	return validBlockSample(data[28:44], 0, bits, channel, upper)
}

//nolint:mnd // Khronos DFD model/channel identifiers, encoded bit lengths and sample limits are fixed wire values.
func blockDescriptorProfile(format uint32) (model, plane, bits, channel byte, upper uint32) {
	switch format {
	case vkBC1, vkBC1SRGB:
		return 128, 8, 63, 0, 0xffffffff
	case vkBC1RGBA, vkBC1RGBASRGB:
		return 128, 8, 63, 1, 0xffffffff
	case vkBC6H:
		return 133, 16, 127, 128, 0x3f800000
	case vkETC2RGB, vkETC2RGBSRGB:
		return 161, 8, 63, 2, 0xffffffff
	case vkETC2RGBA:
		return 161, 16, 63, 2, 0xffffffff
	case vkASTC4x4:
		return 162, 16, 127, 0, 0xffffffff
	default:
		return 134, 16, 127, 0, 0xffffffff
	}
}

func validBlockLayout(data []byte, model, plane, transfer byte) bool {
	return data[12] == model && data[13] <= 1 && data[14] == transfer && data[15] == 0 && data[16] == 3 && data[17] == 3 &&
		data[18] == 0 && data[19] == 0 && data[20] == plane && allZero(data[21:28])
}

func validBlockSample(sample []byte, offset uint16, bits, channel byte, upper uint32) bool {
	return binary.LittleEndian.Uint16(sample) == offset && sample[2] == bits && sample[3] == channel && allZero(sample[4:12]) &&
		binary.LittleEndian.Uint32(sample[12:]) == upper
}
