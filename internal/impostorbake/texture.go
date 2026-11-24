//go:build !js

package impostorbake

import (
	"encoding/binary"
	"fmt"
	"math"
	"math/bits"

	"github.com/karty-game/ashura/internal/modeldata"
)

type pixels struct {
	width, height int
	data          []byte
	srgb          bool
}

func decodeTexture(source modeldata.Texture) (pixels, error) {
	data := source.Data
	if len(data) < 104 || string(data[:12]) != "\xabKTX 20\xbb\r\n\x1a\n" || len(data) > 96<<20 {
		return pixels{}, errUnsupported
	}

	read := func(offset int) uint32 { return binary.LittleEndian.Uint32(data[offset:]) }

	format, width, height, levels := read(12), read(20), read(24), read(40)
	if !validTextureType(data, source.Role, format) || !validTextureDimensions(data, width, height, levels) {
		return pixels{}, fmt.Errorf("requires prepared RGBA8 role-matched KTX2: %w", errUnsupported)
	}

	metadataEnd, err := textureMetadataEnd(data, levels, format)
	if err != nil {
		return pixels{}, err
	}

	if err := validateTextureLevels(data, width, height, levels, metadataEnd); err != nil {
		return pixels{}, err
	}

	offset, length := binary.LittleEndian.Uint64(data[80:]), binary.LittleEndian.Uint64(data[88:])

	return pixels{width: int(width), height: int(height), data: data[offset : offset+length], srgb: source.Role == modeldata.Albedo}, nil
}

func validTextureType(data []byte, role modeldata.TextureRole, format uint32) bool {
	read := func(offset int) uint32 { return binary.LittleEndian.Uint32(data[offset:]) }

	validRole := role <= modeldata.Occlusion &&
		(role == modeldata.Albedo && format == ktxSRGB || role != modeldata.Albedo && format == ktxLinear)
	if !validRole {
		return false
	}

	return read(16) == 1 && read(28) == 0 && read(32) == 0 && read(36) == 1 && read(44) == 0
}

func validTextureDimensions(data []byte, width, height, levels uint32) bool {
	return width > 0 && height > 0 && width <= maxDimension && height <= maxDimension && levels > 0 &&
		int(levels) <= bits.Len32(max(width, height)) && ktxHeader+uint64(levels)*ktxLevelEntry <= uint64(len(data))
}

func (image *pixels) sample(uv [2]float64) vector {
	if image == nil {
		return vector{1, 1, 1}
	}

	column, row := (uv[0]-math.Floor(uv[0]))*float64(image.width)-half, (uv[1]-math.Floor(uv[1]))*float64(image.height)-half
	left, top := int(math.Floor(column)), int(math.Floor(row))
	weightX, weightY := column-math.Floor(column), row-math.Floor(row)

	var result vector

	for deltaY := range 2 {
		for deltaX := range 2 {
			weight := weightX
			if deltaX == 0 {
				weight = 1 - weightX
			}

			if deltaY == 0 {
				weight *= 1 - weightY
			} else {
				weight *= weightY
			}

			pixel := ((wrap(top+deltaY, image.height) * image.width) + wrap(left+deltaX, image.width)) * channels
			for channel := range 3 {
				value := float64(image.data[pixel+channel]) / byteMaximum
				if image.srgb {
					value = toLinear(value)
				}

				result[channel] += weight * value
			}
		}
	}

	return result
}

func wrap(value, size int) int {
	value %= size
	if value < 0 {
		value += size
	}

	return value
}
