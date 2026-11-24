package decaldata

import (
	"bytes"
	"encoding/binary"
	"math"
)

const (
	headerBytes = 16
	magic       = "ASHDCL\x01\x00"
)

// Encode emits an immutable, little-endian GPU record bundle.
func Encode(placements []Placement) ([]byte, error) {
	if len(placements) == 0 || len(placements) > MaxBatch {
		return nil, ErrBatch
	}

	data := make([]byte, headerBytes, headerBytes+len(placements)*RecordBytes)
	copy(data, magic)
	binary.LittleEndian.PutUint32(data[8:], uint32(len(placements))) //nolint:gosec // The build admits a bounded positive count.

	for _, item := range placements {
		projector, err := item.Project()
		if err != nil {
			return nil, err
		}

		record := projector.Record()
		data = append(data, record[:]...)
	}

	return data, nil
}

func Decode(data []byte) ([]byte, error) {
	if len(data) < headerBytes+RecordBytes || len(data) > headerBytes+MaxBatch*RecordBytes ||
		string(data[:8]) != magic || binary.LittleEndian.Uint32(data[12:]) != 0 {
		return nil, ErrBatch
	}

	count := binary.LittleEndian.Uint32(data[8:])
	if count > MaxBatch || len(data) != headerBytes+int(count)*RecordBytes {
		return nil, ErrBatch
	}

	for offset := headerBytes; offset < len(data); offset += RecordBytes {
		if !validRecord(data[offset : offset+RecordBytes]) {
			return nil, ErrBatch
		}
	}

	return bytes.Clone(data[headerBytes:]), nil
}

func validRecord(data []byte) bool {
	var values [20]float32
	for index := range values {
		value := math.Float32frombits(binary.LittleEndian.Uint32(data[index*4:]))
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return false
		}

		values[index] = value
	}

	radius := math.Float32frombits(binary.LittleEndian.Uint32(data[84:]))
	if !(radius > 0 && radius <= MaxCoordinate) || binary.LittleEndian.Uint32(data[MaskOffset:]) == 0 ||
		binary.LittleEndian.Uint32(data[92:]) != 0 {
		return false
	}

	if !(values[12] >= 0 && values[13] >= 0 && values[14] <= 1 && values[15] <= 1 &&
		values[12] < values[14] && values[13] < values[15]) {
		return false
	}

	for _, tint := range values[16:20] {
		if tint < 0 || tint > 1 {
			return false
		}
	}

	return validBounds(values, radius)
}

// A false small sphere could incorrectly cull an otherwise valid projector.
func validBounds(values [20]float32, radius float32) bool {
	const (
		channels        = 4
		radiusTolerance = 1e-6
	)

	var (
		basis         [3]Vector
		lengths       [3]float64
		radiusSquared float64
	)

	for axis := range basis {
		index := axis * channels
		basis[axis] = Vector{values[index], values[index+1], values[index+2]}

		size := length(basis[axis])
		if size < 1e-13 || size > 1.001e5 || abs(values[index+3]) > MaxCoordinate {
			return false
		}

		lengths[axis] = size
		radiusSquared += (Half / size) * (Half / size)
	}

	for first := range basis {
		for second := first + 1; second < len(basis); second++ {
			if math.Abs(float64(dot(basis[first], basis[second]))) > 1e-5*lengths[first]*lengths[second] {
				return false
			}
		}
	}

	return float64(radius) >= math.Sqrt(radiusSquared)*(1-radiusTolerance)
}
