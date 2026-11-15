//go:build !js

//nolint:mnd // RGBE byte encoding uses the specified exponent bias, mantissa range and four channels.
package lightmapbake

import (
	"math"

	"github.com/karty-game/karty-webgpu/pkg/texture"
)

// Atlas is final RGBE data before optional offline native block encoding.
// RGB and RNM share calibrated D units, physical scale one, and exact UV1.
type Atlas struct {
	RGB texture.Data
	RNM [3]texture.Data
}

// EncodeAtlas converts finalized linear results and regenerates predeclared
// chart-confined gutters. Uncovered texels remain zero. No runtime preparation
// is implied by this build-only numeric boundary.
//
//nolint:cyclop,funlen,gocognit,gosec // Validate the complete bounded atlas and every receiver before populating chart gutters.
func EncodeAtlas(result Result, receivers []Receiver, width, height uint32, gutters []uint32) (Atlas, error) {
	var out Atlas

	pixels := uint64(width) * uint64(height)
	if width == 0 || height == 0 || pixels > 2048*2048 || len(receivers) != len(result.Estimates) || len(gutters) != int(pixels) {
		return out, ErrBounds
	}

	makeImage := func() texture.Data {
		return texture.Data{
			Width:      int(width),
			Height:     int(height),
			ColorSpace: texture.Linear,
			Levels:     [][]byte{make([]byte, int(pixels)*4)},
		}
	}

	out.RGB = makeImage()
	if result.Options.Outputs&OutputRNM != 0 {
		for i := range out.RNM {
			out.RNM[i] = makeImage()
		}
	}

	covered := make([]bool, int(pixels))

	for i, receiver := range receivers {
		estimate := result.Estimates[i]
		if receiver.ID != estimate.ReceiverID || uint64(receiver.Pixel) >= pixels || covered[receiver.Pixel] || estimate.Count == 0 {
			return Atlas{}, ErrInvalid
		}

		covered[receiver.Pixel] = true
		if err := encodeRGBE(out.RGB.Levels[0][int(receiver.Pixel)*4:], estimate.Statistics[0]); err != nil {
			return Atlas{}, err
		}

		if result.Options.Outputs&OutputRNM != 0 {
			for j := range out.RNM {
				if err := encodeRGBE(out.RNM[j].Levels[0][int(receiver.Pixel)*4:], estimate.RNM[j]); err != nil {
					return Atlas{}, err
				}
			}
		}
	}

	applyGutters := func(image *texture.Data) error {
		data := image.Levels[0]

		for pixel, owner := range gutters {
			if covered[pixel] || owner == math.MaxUint32 {
				continue
			}

			if uint64(owner) >= pixels || !covered[owner] {
				return ErrInvalid
			}

			copy(data[pixel*4:pixel*4+4], data[int(owner)*4:int(owner)*4+4])
		}

		return nil
	}
	if err := applyGutters(&out.RGB); err != nil {
		return Atlas{}, err
	}

	if result.Options.Outputs&OutputRNM != 0 {
		for i := range out.RNM {
			if err := applyGutters(&out.RNM[i]); err != nil {
				return Atlas{}, err
			}
		}
	}

	return out, nil
}

func encodeRGBE(target []byte, value Vec3) error {
	peak := float64(0)

	for _, channel := range value {
		v := float64(channel)
		if v < 0 || math.IsNaN(v) || math.IsInf(v, 0) {
			return ErrInvalid
		}

		peak = max(peak, v)
	}

	if peak == 0 {
		clear(target[:4])

		return nil
	}

	mantissa, exponent := math.Frexp(peak)
	if mantissa == .5 {
		exponent--
	}

	if exponent < -127 || exponent > 127 {
		return ErrBounds
	}

	factor := math.Ldexp(255, -exponent)
	for i, channel := range value {
		target[i] = byte(max(0, min(255, math.Round(float64(channel)*factor))))
	}

	target[3] = byte(exponent + 128)

	return nil
}
