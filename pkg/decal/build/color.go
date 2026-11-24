//go:build !js

package decalbuild

import (
	"math"

	"github.com/karty-game/ashura/internal/decaldata"
	"github.com/karty-game/ashura/pkg/assetbuild"
)

const (
	rgbaChannels = 4
	channelMax   = 255
)

func paintPixel(pixel []byte, position, normal decaldata.Vector, marks []bakedMark) {
	var color [3]float64

	applied := false

	for _, mark := range marks {
		u, v, inside := mark.projection.UV(position, normal)
		if !inside {
			continue
		}

		stamp := filter(mark.image, float64(u), float64(v))
		tint := mark.projection.Tint

		opacity := stamp[3] * float64(tint[3])
		if opacity == 0 {
			continue
		}

		if !applied {
			for channel := range color {
				color[channel] = linear(pixel[channel])
			}

			applied = true
		}

		for channel := range color {
			color[channel] = color[channel]*(1-opacity) + stamp[channel]*float64(tint[channel])*float64(tint[3])
		}
	}

	if applied {
		for channel := range color {
			pixel[channel] = srgb(color[channel])
		}
	}
}

func filter(image assetbuild.Image, u, v float64) [4]float64 {
	sourceU, sourceV := u*float64(image.Width)-decaldata.Half, v*float64(image.Height)-decaldata.Half
	left, top := math.Floor(sourceU), math.Floor(sourceV)

	var result [4]float64

	for row := range 2 {
		weightY := [2]float64{1 - (sourceV - top), sourceV - top}[row]

		for column := range 2 {
			weightX := [2]float64{1 - (sourceU - left), sourceU - left}[column]
			sourceX := max(0, min(image.Width-1, int(left)+column))
			sourceY := max(0, min(image.Height-1, int(top)+row))
			index := (sourceY*image.Width + sourceX) * rgbaChannels
			pixel := image.Levels[0][index:]
			alpha := float64(pixel[3]) / channelMax

			weight := weightX * weightY * alpha
			for channel := range 3 {
				result[channel] += linear(pixel[channel]) * weight
			}

			result[3] += weight
		}
	}

	return result
}

//nolint:mnd // IEC 61966-2-1 sRGB transfer constants and byte range.
func linear(value byte) float64 {
	channel := float64(value) / 255
	if channel <= .04045 {
		return channel / 12.92
	}

	return math.Pow((channel+.055)/1.055, 2.4)
}

//nolint:mnd // IEC 61966-2-1 sRGB transfer constants and byte range.
func srgb(value float64) byte {
	value = max(0, min(1, value))
	if value <= .0031308 {
		return byte(math.Round(value * 12.92 * 255))
	}

	return byte(math.Round((1.055*math.Pow(value, 1.0/2.4) - .055) * 255))
}
