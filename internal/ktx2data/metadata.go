package ktx2data

import (
	"bytes"
	"encoding/binary"
)

const (
	descriptorSize = 92
	wordBytes      = 4
	alphaIndex     = 3
	paddingMask    = wordBytes - 1
)

// Admission checks cover prepared pixel and native block profiles; metadata never rewrites payloads.
func metadataEnd(encoded []byte, indexEnd uint64, format uint32) (uint64, error) {
	read := func(offset int) uint32 { return binary.LittleEndian.Uint32(encoded[offset:]) }

	offset, length := uint64(read(48)), uint64(read(52))

	expectedSize := descriptorLength(format)

	if offset < indexEnd || offset%4 != 0 || length != expectedSize || offset > uint64(len(encoded)) ||
		length > uint64(len(encoded))-offset {
		return 0, ErrBounds
	}

	descriptor := encoded[offset : offset+length]

	if !validDescriptor(descriptor, format) {
		return 0, ErrFormat
	}

	end := offset + length
	kvOffset, kvLength := uint64(read(56)), uint64(read(60))

	if binary.LittleEndian.Uint64(encoded[64:]) != 0 || binary.LittleEndian.Uint64(encoded[72:]) != 0 {
		return 0, ErrFormat
	}

	if kvLength == 0 {
		if kvOffset != 0 {
			return 0, ErrBounds
		}

		return end, nil
	}

	if kvOffset < end || kvOffset%4 != 0 || kvOffset > uint64(len(encoded)) || kvLength > uint64(len(encoded))-kvOffset {
		return 0, ErrBounds
	}

	if err := validateKeys(encoded[kvOffset : kvOffset+kvLength]); err != nil {
		return 0, err
	}

	return kvOffset + kvLength, nil
}

func validateKeys(data []byte) error {
	for len(data) > 0 {
		if len(data) < wordBytes {
			return ErrBounds
		}

		size := uint64(binary.LittleEndian.Uint32(data))

		padded := (size + paddingMask) &^ uint64(paddingMask)
		if size < 2 || padded > uint64(len(data))-wordBytes {
			return ErrBounds
		}

		entry := data[4 : 4+size]

		split := bytes.IndexByte(entry, 0)
		if split < 1 {
			return ErrBounds
		}

		key, value := string(entry[:split]), string(bytes.TrimRight(entry[split+1:], "\x00"))
		if key == "KTXorientation" && value != "rd" || key == "KTXswizzle" && value != "rgba" {
			return ErrFormat
		}

		data = data[4+padded:]
	}

	return nil
}

func allZero(data []byte) bool {
	for _, value := range data {
		if value != 0 {
			return false
		}
	}

	return true
}

func validDescriptor(descriptor []byte, format uint32) bool {
	if blockFormat(format) {
		return validBlockDescriptor(descriptor, format)
	}

	transfer := byte(1)
	if srgbFormat(format) {
		transfer = 2
	}

	if binary.LittleEndian.Uint32(descriptor) != descriptorSize || binary.LittleEndian.Uint32(descriptor[4:]) != 0 ||
		binary.LittleEndian.Uint16(descriptor[8:]) != 2 ||
		binary.LittleEndian.Uint16(descriptor[10:]) != descriptorSize-4 ||
		descriptor[12] != 1 ||
		descriptor[14] != transfer ||
		descriptor[15] != 0 ||
		binary.LittleEndian.Uint32(descriptor[16:]) != 0 ||
		descriptor[20] != 4 ||
		!allZero(descriptor[21:28]) {
		return false
	}

	return validSamples(descriptor)
}

func validSamples(descriptor []byte) bool {
	for channel := range 4 {
		sample := descriptor[28+channel*16 : 28+(channel+1)*16]

		kind := byte(channel)
		if channel == alphaIndex {
			kind = 15
		}

		if binary.LittleEndian.Uint16(sample) != uint16(channel*8) || sample[2] != 7 || sample[3]&15 != kind || sample[3]&0xe0 != 0 ||
			!allZero(sample[4:12]) ||
			binary.LittleEndian.Uint32(sample[12:]) != 255 {
			return false
		}
	}

	return true
}
