//go:build !js

package assetbuild

import (
	"encoding/binary"
	"image"

	"github.com/karty-game/karty-webgpu/internal/ktx2data"
	"github.com/karty-game/karty-webgpu/pkg/texture"
)

// PrepareNativeSSBumpRGBA8 writes a canonical linear RGBA8 container without
// changing any authored channel or mip byte. It performs no compression,
// transfer conversion, resizing or mip generation and accepts unrelated alpha.
func PrepareNativeSSBumpRGBA8(source []byte) ([]byte, error) {
	data, err := ktx2data.Decode(source)
	if err != nil || data.Format != texture.RGBA8 || data.ColorSpace != texture.Linear {
		return nil, errBlockCompression
	}

	levels := make([]*image.NRGBA, len(data.Levels))

	width, height := data.Width, data.Height
	for index, pixels := range data.Levels {
		levels[index] = &image.NRGBA{Pix: pixels, Stride: width * imageChannels, Rect: image.Rect(0, 0, width, height)}
		width, height = max(1, width>>1), max(1, height>>1)
	}

	output := encodeKTX(levels, false)
	descriptor := int(binary.LittleEndian.Uint32(output[48:]))
	// VK37 expresses linear alpha implicitly; all pixel alpha bytes stay intact.
	output[descriptor+28+3*16+3] = 15

	return output, nil
}
