//go:build !js

//nolint:mnd,gosec // Fixed RGBA/RGBE/KTX2 wire values and offsets already bounded by KTX admission.
package assetbuild

import (
	"bytes"
	"encoding/binary"
	"math"

	"github.com/karty-game/karty-webgpu/internal/ktx2data"
	"github.com/karty-game/karty-webgpu/internal/texturedata"
	"github.com/karty-game/karty-webgpu/pkg/texture"
)

// PrepareLDRLightmapCoefficients converts one RGB or three RNM Karty RGBE
// textures to opaque RGBA8 using one complete-family maximum. The returned
// physical scale preserves the source units; 8-bit storage introduces ordinary
// quantization but no exposure clipping. Transfer conversion runs only offline.
func PrepareLDRLightmapCoefficients(sources [][]byte, scale float32, space texture.ColorSpace) ([][]byte, float32, error) {
	if (len(sources) != 1 && len(sources) != 3) || scale <= 0 || math.IsNaN(float64(scale)) || math.IsInf(float64(scale), 0) ||
		(space != texture.Linear && space != texture.SRGB) {
		return nil, 0, errBlockCompression
	}

	images, maxima, err := ldrCoefficientImages(sources)
	if err != nil {
		return nil, 0, err
	}

	maximum := float32(0)

	for exponent, mantissa := range maxima {
		if exponent != 0 && mantissa != 0 {
			maximum = max(maximum, ldrRGBEValue(mantissa, byte(exponent)))
		}
	}

	physicalScale := scale
	if maximum != 0 {
		physicalScale = float32(float64(scale) * float64(maximum))
	}

	if physicalScale <= 0 || math.IsInf(float64(physicalScale), 0) {
		return nil, 0, errBlockCompression
	}

	lookup := ldrCoefficientLookup(maxima, maximum, space)

	output := make([][]byte, len(sources))
	for index, source := range sources {
		data := bytes.Clone(source)
		ldrCoefficientTransfer(data, space)
		offset := int(binary.LittleEndian.Uint64(data[80:]))
		pixels := data[offset : offset+len(images[index].Levels[0])]

		input := images[index].Levels[0]
		for pixel := 0; pixel < len(input); pixel += 4 {
			row := int(input[pixel+3]) << 8
			pixels[pixel] = lookup[row+int(input[pixel])]
			pixels[pixel+1] = lookup[row+int(input[pixel+1])]
			pixels[pixel+2] = lookup[row+int(input[pixel+2])]
			pixels[pixel+3] = 255
		}

		output[index] = data
	}

	return output, physicalScale, nil
}

func ldrCoefficientImages(sources [][]byte) ([]texturedata.Data, [256]byte, error) {
	images := make([]texturedata.Data, len(sources))

	var maxima [256]byte

	for index, source := range sources {
		image, err := ktx2data.Decode(source)
		if err != nil || image.Format != texture.RGBA8 || image.ColorSpace != texture.Linear || len(image.Levels) != 1 ||
			(index != 0 && (image.Width != images[0].Width || image.Height != images[0].Height)) {
			return nil, maxima, errBlockCompression
		}

		images[index] = image

		pixels := image.Levels[0]
		for pixel := 0; pixel < len(pixels); pixel += 4 {
			exponent := pixels[pixel+3]
			if exponent != 0 {
				maxima[exponent] = max(maxima[exponent], pixels[pixel], pixels[pixel+1], pixels[pixel+2])
			}
		}
	}

	return images, maxima, nil
}

func ldrRGBEValue(mantissa, exponent byte) float32 {
	pixel := [4]byte{mantissa, 0, 0, exponent}

	return decodeBlockRGBE(pixel[:], 0)
}

func ldrCoefficientLookup(maxima [256]byte, maximum float32, space texture.ColorSpace) *[1 << 16]byte {
	lookup := new([1 << 16]byte)
	if maximum == 0 {
		return lookup
	}

	for exponent, largest := range maxima {
		if exponent == 0 {
			continue
		}

		for mantissa := 1; mantissa <= int(largest); mantissa++ {
			value := float64(ldrRGBEValue(byte(mantissa), byte(exponent))) / float64(maximum)
			if space == texture.SRGB {
				value = toSRGB(value)
			}

			lookup[exponent<<8|mantissa] = byte(math.Round(value * 255))
		}
	}

	return lookup
}

func ldrCoefficientTransfer(data []byte, space texture.ColorSpace) {
	format, transfer := uint32(37), byte(1)
	if space == texture.SRGB {
		format, transfer = 43, 2
	}

	binary.LittleEndian.PutUint32(data[12:], format)
	descriptor := int(binary.LittleEndian.Uint32(data[48:]))
	data[descriptor+13] = 1
	data[descriptor+14] = transfer

	data[descriptor+28+3*16+3] = 15
	if space == texture.SRGB {
		data[descriptor+28+3*16+3] = 31
	}
}
