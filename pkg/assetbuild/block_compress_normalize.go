//go:build !js

//nolint:mnd,gosec // RGBE exponent bias/range and previously admitted KTX2 level offsets.
package assetbuild

import (
	"encoding/binary"
	"math"
)

// NormalizeRGBERNM uses one exact power-of-two exposure for all three page
// coefficients. The highest stored exponent targets 12 (roughly 2048..4096),
// keeping dim coefficients out of BC6H half-float subnormals. Only RGBE exponent
// bytes change; the returned scale compensates before any lossy encoding.
func NormalizeRGBERNM(sources [3][]byte, scale float32) ([3][]byte, float32, error) {
	var result [3][]byte

	values, scale, err := NormalizeRGBECoefficients(sources[:], scale)
	if err != nil {
		return result, 0, err
	}

	copy(result[:], values)

	return result, scale, nil
}

// NormalizeRGBECoefficients applies one exact exposure to a complete RGB or RNM family.
//
//nolint:cyclop // Validate every numeric channel of the complete coefficient family before conversion.
func NormalizeRGBECoefficients(sources [][]byte, scale float32) ([][]byte, float32, error) {
	output := make([][]byte, len(sources))
	if len(sources) != 1 && len(sources) != 3 {
		return nil, 0, errBlockCompression
	}

	if scale <= 0 || math.IsInf(float64(scale), 0) || math.IsNaN(float64(scale)) {
		return output, 0, errBlockCompression
	}

	minimum, maximum := 255, 0
	width, height := 0, 0

	for _, source := range sources {
		image, err := blockInput(source)
		if err != nil || len(image.Levels) != 1 || width != 0 && (image.Width != width || image.Height != height) {
			return output, 0, errBlockCompression
		}

		width, height = image.Width, image.Height
		for offset := 3; offset < len(image.Levels[0]); offset += 4 {
			exponent := int(image.Levels[0][offset])
			if exponent != 0 {
				minimum, maximum = min(minimum, exponent), max(maximum, exponent)
			}
		}
	}

	if maximum-minimum > 253 {
		return output, 0, errBlockCompression
	}

	shift := 0
	if maximum != 0 {
		shift = min(max(140-maximum, 1-minimum), 254-maximum)
	}

	adjustedScale := float32(math.Ldexp(float64(scale), -shift))
	if adjustedScale <= 0 || math.IsInf(float64(adjustedScale), 0) {
		return output, 0, errBlockCompression
	}

	for index, source := range sources {
		output[index] = append([]byte(nil), source...)

		level := int(binary.LittleEndian.Uint64(source[80:]))
		for offset := level + 3; offset < level+width*height*4; offset += 4 {
			if exponent := int(output[index][offset]); exponent != 0 {
				output[index][offset] = byte(exponent + shift)
			}
		}
	}

	return output, adjustedScale, nil
}
