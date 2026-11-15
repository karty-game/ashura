//go:build !js

package assetbuild

import (
	"image"
	"image/color"
	"math"
)

const (
	byteMaximum          = 255
	normalRange          = 2
	colorChannels        = 3
	halfRange            = .5
	srgbLinearThreshold  = .04045
	srgbEncodedThreshold = .0031308
	srgbLinearScale      = 12.92
	srgbOffset           = .055
	srgbScale            = 1.055
	srgbGamma            = 2.4
)

func mipmaps(base *image.NRGBA, usage string, limit int) []*image.NRGBA {
	levels := []*image.NRGBA{base}
	for source := base; (source.Bounds().Dx() > 1 || source.Bounds().Dy() > 1) && (limit == 0 || len(levels) < limit); {
		width, height := max(source.Bounds().Dx()>>1, 1), max(source.Bounds().Dy()>>1, 1)

		next := image.NewNRGBA(image.Rect(0, 0, width, height))
		for y := range height {
			for x := range width {
				next.SetNRGBA(x, y, filterPixel(source, x, y, width, height, usage))
			}
		}

		levels = append(levels, next)
		source = next
	}

	return levels
}

func filterPixel(source *image.NRGBA, x, y, width, height int, usage string) color.NRGBA {
	var sum [4]float64

	startX, endX := x*source.Bounds().Dx()/width, (x+1)*source.Bounds().Dx()/width
	startY, endY := y*source.Bounds().Dy()/height, (y+1)*source.Bounds().Dy()/height

	samples := float64((endX - startX) * (endY - startY))
	for sampleY := startY; sampleY < endY; sampleY++ {
		for sampleX := startX; sampleX < endX; sampleX++ {
			sample := source.NRGBAAt(sampleX, sampleY)

			values := [4]byte{sample.R, sample.G, sample.B, sample.A}
			for channel, value := range values {
				decoded := float64(value) / byteMaximum

				if channel < colorChannels {
					switch usage {
					case UsageNormal:
						decoded = decoded*normalRange - 1
					case UsageAlbedo:
						decoded = toLinear(decoded)
					}
				}

				sum[channel] += decoded / samples
			}
		}
	}

	if usage == UsageNormal {
		length := math.Sqrt(sum[0]*sum[0] + sum[1]*sum[1] + sum[2]*sum[2])
		if length > 0 {
			for channel := range colorChannels {
				sum[channel] /= length
			}
		} else {
			sum[2] = 1
		}
	}

	var encoded [4]byte

	for channel, value := range sum {
		if channel < colorChannels {
			switch usage {
			case UsageNormal:
				value = value*halfRange + halfRange
			case UsageAlbedo:
				value = toSRGB(value)
			}
		}

		encoded[channel] = byte(math.Round(max(0, min(byteMaximum, value*byteMaximum))))
	}

	return color.NRGBA{R: encoded[0], G: encoded[1], B: encoded[2], A: encoded[3]}
}

func toLinear(value float64) float64 {
	if value <= srgbLinearThreshold {
		return value / srgbLinearScale
	}

	return math.Pow((value+srgbOffset)/srgbScale, srgbGamma)
}
func toSRGB(value float64) float64 {
	if value <= srgbEncodedThreshold {
		return value * srgbLinearScale
	}

	return srgbScale*math.Pow(value, 1/srgbGamma) - srgbOffset
}
