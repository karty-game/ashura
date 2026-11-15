//nolint:mnd,gosec // Fixed UV ABI; counts and lengths are admitted before bounded integer conversions.
package leveldata

import (
	"crypto/sha256"
	"encoding/binary"
	"math"
)

const (
	uvHeader      = 80
	maxUVParts    = 4096
	maxUVVertices = 1 << 20
	maxUVBytes    = uvHeader + maxUVParts*4 + maxUVVertices*8
)

// UVLayout is a lightweight SHA-bound secondary UV stream with no bake scene payload.
type UVLayout struct {
	digest [32]byte
	parts  [][]byte
}

func (layout *UVLayout) ModelDigest() [32]byte {
	if layout == nil {
		return [32]byte{}
	}

	return layout.digest
}
func (layout *UVLayout) PartCount() int {
	if layout == nil {
		return 0
	}

	return len(layout.parts)
}

// PartUV returns immutable GPU-ready UV1 bytes; out-of-range indices return nil.
func (layout *UVLayout) PartUV(index int) []byte {
	if layout == nil || index < 0 || index >= len(layout.parts) {
		return nil
	}

	return layout.parts[index]
}

// EncodeUV creates a GPU-ready layout independently of BVH/receiver/transport data.
func EncodeUV(digest [32]byte, parts [][]byte) ([]byte, error) {
	if len(parts) == 0 || len(parts) > maxUVParts {
		return nil, ErrFormat
	}

	size, vertices := uvHeader, 0

	for _, part := range parts {
		if !validUV(part) {
			return nil, ErrFormat
		}

		vertices += len(part) / 8

		size += 4 + len(part)
		if size > MaxPayloadBytes || vertices > maxUVVertices {
			return nil, ErrFormat
		}
	}

	out := make([]byte, size)
	copy(out, "KARTUV1\x00")
	binary.LittleEndian.PutUint32(out[8:], 1)
	binary.LittleEndian.PutUint32(out[12:], uint32(len(parts)))
	copy(out[16:48], digest[:])

	offset := uvHeader
	for _, part := range parts {
		binary.LittleEndian.PutUint32(out[offset:], uint32(len(part)))
		offset += 4
		offset += copy(out[offset:], part)
	}

	sum := uvChecksum(out)
	copy(out[48:80], sum[:])

	return out, nil
}

// DecodeUV validates and takes an immutable owned copy before exposing its streams.
func DecodeUV(data []byte) (*UVLayout, error) {
	if err := validateUV(data); err != nil {
		return nil, err
	}

	return readUV(append([]byte(nil), data...)), nil
}

// AdmitUVOwned validates immutable bytes already owned by an internal consumer.
// It retains their storage without another complete UV copy.
func AdmitUVOwned(data []byte) (*UVLayout, error) {
	if err := validateUV(data); err != nil {
		return nil, err
	}

	return readUV(data), nil
}

func validateUV(data []byte) error {
	if len(data) < uvHeader || len(data) > maxUVBytes || string(data[:8]) != "KARTUV1\x00" ||
		binary.LittleEndian.Uint32(data[8:]) != 1 {
		return ErrFormat
	}

	count := binary.LittleEndian.Uint32(data[12:])
	if count == 0 || count > maxUVParts {
		return ErrFormat
	}

	var expected [32]byte
	copy(expected[:], data[48:80])

	if uvChecksum(data) != expected {
		return ErrFormat
	}

	offset, vertices := uvHeader, 0
	for range count {
		if offset > len(data)-4 {
			return ErrFormat
		}

		size := uint64(binary.LittleEndian.Uint32(data[offset:]))

		offset += 4
		if size > uint64(len(data)-offset) || !validUV(data[offset:offset+int(size)]) {
			return ErrFormat
		}

		offset += int(size)

		vertices += int(size) / 8
		if vertices > maxUVVertices {
			return ErrFormat
		}
	}

	if offset != len(data) {
		return ErrFormat
	}

	return nil
}

func readUV(owned []byte) *UVLayout {
	count := binary.LittleEndian.Uint32(owned[12:])
	layout := &UVLayout{parts: make([][]byte, count)}
	copy(layout.digest[:], owned[16:48])

	offset := uvHeader
	for index := range layout.parts {
		size := int(binary.LittleEndian.Uint32(owned[offset:]))
		offset += 4
		layout.parts[index] = owned[offset : offset+size : offset+size]
		offset += size
	}

	return layout
}

func validUV(data []byte) bool {
	if len(data) == 0 || len(data)%8 != 0 || len(data) > 8*maxUVVertices {
		return false
	}

	for offset := 0; offset < len(data); offset += 4 {
		value := math.Float32frombits(binary.LittleEndian.Uint32(data[offset:]))
		if !finite(value) || value < 0 || value > 1 {
			return false
		}
	}

	return true
}
func uvChecksum(data []byte) [32]byte {
	hash := sha256.New()
	_, _ = hash.Write(data[:48])
	_, _ = hash.Write(data[80:])

	var out [32]byte
	copy(out[:], hash.Sum(nil))

	return out
}
