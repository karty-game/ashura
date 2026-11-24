//go:build !js

//nolint:mnd // RGBE bytes and prepared receiver/padding records have fixed ABI offsets.
package lightmapbuild

import (
	"context"
	"encoding/binary"
	"math"

	"github.com/karty-game/ashura/internal/lightmapdata"
	"github.com/karty-game/ashura/pkg/texture"
)

func denoiseDecode(pixel []byte, scale float32) [3]float64 {
	if pixel[3] == 0 {
		return [3]float64{}
	}

	factor := math.Ldexp(float64(scale)/255, int(pixel[3])-128)

	return [3]float64{float64(pixel[0]) * factor, float64(pixel[1]) * factor, float64(pixel[2]) * factor}
}

func denoiseEncode(pixel []byte, value [3]float64, scale float32) error {
	peak := max(value[0], value[1], value[2])
	if math.IsInf(peak, 0) || math.IsNaN(peak) || peak > math.MaxFloat32 {
		return errDenoise
	}

	if peak <= 0 {
		clear(pixel[:4])

		return nil
	}

	peak /= float64(scale)

	mantissa, exponent := math.Frexp(peak)
	if mantissa == .5 {
		exponent--
	}

	exponent = max(-127, min(127, exponent))

	factor := math.Ldexp(255/float64(scale), -exponent)
	for axis := range 3 {
		pixel[axis] = byte(max(0, min(255, math.Round(value[axis]*factor))))
	}

	pixel[3] = byte(exponent + 128)

	return nil
}

func denoiseDifference(ctx context.Context, document *lightmapdata.Document, total, direct RNMResult) ([]float32, error) {
	pixels, width := int(document.Width)*int(document.Height), int(document.Width)

	output := make([]float32, pixels*denoiseChannels)
	for pixel := range pixels {
		if pixel%width == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}

		if denoiseNumber(document.Receivers[pixel*lightmapdata.ReceiverStride+12:]) == 0 {
			continue
		}

		for basis := range 3 {
			all := denoiseDecode(total.Textures[basis].Levels[0][pixel*4:], total.Scale)

			directional := denoiseDecode(direct.Textures[basis].Levels[0][pixel*4:], direct.Scale)
			for axis := range 3 {
				value := max(0, all[axis]-directional[axis])
				if value > math.MaxFloat32 {
					return nil, errDenoise
				}

				output[pixel*denoiseChannels+basis*3+axis] = float32(value)
			}
		}
	}

	return output, nil
}

func denoiseResolve(
	ctx context.Context,
	document *lightmapdata.Document,
	total, direct RNMResult,
	indirect []float32,
) (RNMResult, error) {
	result := total

	width, height := int(document.Width), int(document.Height)
	for basis := range 3 {
		result.Textures[basis] = texture.Data{Width: width, Height: height, ColorSpace: texture.Linear,
			Levels: [][]byte{make([]byte, width*height*4)}}
	}

	for pixel := range width * height {
		if pixel%width == 0 {
			if err := ctx.Err(); err != nil {
				return RNMResult{}, err
			}
		}

		owner := binary.LittleEndian.Uint32(document.Padding[pixel*4:])
		if owner == math.MaxUint32 {
			continue
		}

		for basis := range 3 {
			value := denoiseDecode(direct.Textures[basis].Levels[0][int(owner)*4:], direct.Scale)
			for axis := range 3 {
				value[axis] += float64(indirect[int(owner)*denoiseChannels+basis*3+axis])
			}

			if err := denoiseEncode(result.Textures[basis].Levels[0][pixel*4:], value, total.Scale); err != nil {
				return RNMResult{}, err
			}
		}
	}

	return result, nil
}
