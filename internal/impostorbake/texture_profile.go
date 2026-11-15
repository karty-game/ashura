//go:build !js

package impostorbake

import "encoding/binary"

const (
	ktxHeader              = 80
	ktxLevelEntry          = 24
	ktxDescriptor          = 92
	ktxSRGB                = 43
	ktxLinear              = 37
	ktxDescriptorVersion   = 2
	ktxDescriptorBlockSize = 88
)

// The GLB compiler emits this exact uncompressed RGBA8 profile. Reject alternate
// swizzles/orientations and unsupported metadata rather than silently reinterpret.
func textureMetadataEnd(data []byte, levels, format uint32) (uint64, error) {
	read := func(offset int) uint32 { return binary.LittleEndian.Uint32(data[offset:]) }

	offset, length := uint64(read(48)), uint64(read(52))
	if offset < ktxHeader+uint64(levels)*ktxLevelEntry || offset%channels != 0 || length != ktxDescriptor || offset > uint64(len(data)) ||
		length > uint64(len(data))-offset {
		return 0, errUnsupported
	}

	if read(56) != 0 || read(60) != 0 || binary.LittleEndian.Uint64(data[64:]) != 0 || binary.LittleEndian.Uint64(data[72:]) != 0 {
		return 0, errUnsupported
	}

	descriptor := data[offset : offset+length]
	if !validTextureDescriptor(descriptor, format) {
		return 0, errUnsupported
	}

	return offset + length, nil
}

func validTextureDescriptor(descriptor []byte, format uint32) bool {
	transfer := byte(1)
	if format == ktxSRGB {
		transfer = 2
	}

	headerValid := binary.LittleEndian.Uint32(descriptor) == ktxDescriptor && binary.LittleEndian.Uint32(descriptor[4:]) == 0

	versionValid := binary.LittleEndian.Uint16(descriptor[8:]) == ktxDescriptorVersion &&
		binary.LittleEndian.Uint16(descriptor[10:]) == ktxDescriptorBlockSize
	if !headerValid || !versionValid {
		return false
	}

	encodingValid := descriptor[12] == 1 && descriptor[14] == transfer && descriptor[15] == 0

	storageValid := binary.LittleEndian.Uint32(descriptor[16:]) == 0 && descriptor[20] == channels && zero(descriptor[21:28])
	if !encodingValid || !storageValid {
		return false
	}

	return validTextureSamples(descriptor)
}

func validTextureSamples(descriptor []byte) bool {
	for channel := range 4 {
		value := descriptor[28+channel*16 : 28+(channel+1)*16]

		kind := byte(channel)
		if channel == triangleVertices {
			kind = 15
		}

		if binary.LittleEndian.Uint16(value) != uint16(channel*8) || value[2] != 7 || value[3]&15 != kind || value[3]&0xe0 != 0 ||
			!zero(value[4:12]) || binary.LittleEndian.Uint32(value[12:]) != 255 {
			return false
		}
	}

	return true
}

func validateTextureLevels(data []byte, width, height, levels uint32, metadataEnd uint64) error {
	ranges := make([][2]uint64, 0, levels)
	for level := range levels {
		entry := ktxHeader + level*ktxLevelEntry
		offset, length := binary.LittleEndian.Uint64(data[entry:]), binary.LittleEndian.Uint64(data[entry+8:])
		uncompressed := binary.LittleEndian.Uint64(data[entry+16:])

		expected := uint64(max(1, width>>level)) * uint64(max(1, height>>level)) * channels
		if offset < metadataEnd || offset%4 != 0 || offset > uint64(len(data)) || length > uint64(len(data))-offset || length != expected ||
			uncompressed != length {
			return errBounds
		}

		for _, prior := range ranges {
			if offset < prior[1] && offset+length > prior[0] {
				return errBounds
			}
		}

		ranges = append(ranges, [2]uint64{offset, offset + length})
	}

	return nil
}

func zero(values []byte) bool {
	for _, value := range values {
		if value != 0 {
			return false
		}
	}

	return true
}
