//go:build !js

//nolint:mnd // BC1 uses 4x4 texels, 8-byte blocks, RGB565 endpoints and two-bit selectors.
package assetbuild

import (
	"context"
	"encoding/binary"

	"github.com/karty-game/ashura/internal/texturedata"
)

func patchBC1Level(ctx context.Context, width, height int, pixels, blocks []byte) error {
	if width < 1 || height < 1 || width > 4096 || height > 4096 || len(pixels) != width*height*imageChannels {
		return errBlockCompression
	}

	rowBytes, rows := texturedata.Layout(texturedata.BC1, width, height)
	if len(blocks) != rowBytes*rows {
		return errBlockCompression
	}

	for row := range rows {
		if err := ctx.Err(); err != nil {
			return err
		}

		for column := range rowBytes / 8 {
			var source [16][4]byte

			for y := range 4 {
				for x := range 4 {
					offset := (min(row*4+y, height-1)*width + min(column*4+x, width-1)) * imageChannels
					copy(source[y*4+x][:], pixels[offset:offset+imageChannels])
				}
			}

			offset := row*rowBytes + column*8
			patchBC1Block(blocks[offset:offset+8], source)
		}
	}

	return nil
}

func patchBC1Block(block []byte, source [16][4]byte) {
	covered := 0

	for _, pixel := range source {
		if pixel[3] >= 128 {
			covered++
		}
	}

	if covered == 0 {
		binary.LittleEndian.PutUint64(block, 0xffffffff00000000)

		return
	}

	first, second := binary.LittleEndian.Uint16(block), binary.LittleEndian.Uint16(block[2:])

	selectors := binary.LittleEndian.Uint32(block[4:])
	if covered == 16 && (first > second || !bc1HasTransparentSelector(selectors)) {
		return // Preserve the WASM encoder's opaque blocks whenever alpha is safe.
	}

	if first > second {
		first, second = second, first
	}

	palette := bc1MaskPalette(first, second)
	selectors = 0

	for index, pixel := range source {
		selector := uint32(3)
		if pixel[3] >= 128 {
			selector = bc1NearestColor(pixel, palette)
		}

		selectors |= selector << (2 * index)
	}

	binary.LittleEndian.PutUint16(block, first)
	binary.LittleEndian.PutUint16(block[2:], second)
	binary.LittleEndian.PutUint32(block[4:], selectors)
}

func bc1HasTransparentSelector(selectors uint32) bool {
	for range 16 {
		if selectors&3 == 3 {
			return true
		}

		selectors >>= 2
	}

	return false
}

func bc1MaskPalette(first, second uint16) [3][3]int {
	var palette [3][3]int

	for index, endpoint := range [2]uint16{first, second} {
		red, green, blue := int(endpoint>>11), int(endpoint>>5&63), int(endpoint&31)
		palette[index] = [3]int{red<<3 | red>>2, green<<2 | green>>4, blue<<3 | blue>>2}
	}

	for channel := range 3 {
		palette[2][channel] = (palette[0][channel] + palette[1][channel]) / 2
	}

	return palette
}

func bc1NearestColor(pixel [4]byte, palette [3][3]int) uint32 {
	best, errorSquared := uint32(0), int(^uint(0)>>1)

	for index, color := range palette {
		distance := 0

		for channel := range 3 {
			delta := int(pixel[channel]) - color[channel]
			distance += delta * delta
		}

		if distance < errorSquared {
			best, errorSquared = uint32(index), distance
		}
	}

	return best
}
