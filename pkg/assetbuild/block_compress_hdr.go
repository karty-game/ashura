//go:build !js

package assetbuild

import (
	"context"
	"encoding/binary"
	"math"

	"github.com/karty-game/karty-webgpu/internal/texturedata"
)

// CompressRGBERNM decodes Karty RGBE numerics offline into normalized linear
// HDR and produces GPU-native BC6H unsigned-float KTX2. Keep the original page
// Scale: it is not applied to these normalized coefficients. No resize occurs.
func CompressRGBERNM(ctx context.Context, source []byte, options BlockCompressionOptions) ([]byte, error) {
	return compressRGBEGo(ctx, source, options)
}

func rgbeFloatPixels(image texturedata.Data) ([]byte, error) {
	// Admit the float buffer before allocation, matching the guest request bound.
	if image.Width <= 0 || image.Height <= 0 || uint64(image.Width)*uint64(image.Height)*16 > 128<<20 ||
		len(image.Levels) != 1 || uint64(len(image.Levels[0])) != uint64(image.Width)*uint64(image.Height)*4 {
		return nil, errBlockCompression
	}

	output := make([]byte, image.Width*image.Height*16)
	put := func(offset int, value uint32) { binary.LittleEndian.PutUint32(output[offset:], value) }

	for offset := 0; offset < len(image.Levels[0]); offset += 4 {
		pixel := image.Levels[0][offset : offset+4]
		for channel := range 3 {
			value := decodeBlockRGBE(pixel, channel)
			if value > 65504 || math.IsInf(float64(value), 0) || math.IsNaN(float64(value)) {
				return nil, errBlockCompression
			}

			put(offset*4+channel*4, math.Float32bits(value))
		}

		put(offset*4+12, math.Float32bits(1))
	}

	return output, nil
}

func decodeBlockRGBE(pixel []byte, channel int) float32 {
	if pixel[3] == 0 {
		return 0
	}

	return float32(math.Ldexp(float64(pixel[channel])/255, int(pixel[3])-128))
}
