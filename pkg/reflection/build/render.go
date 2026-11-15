//go:build !js

//nolint:mnd // Panorama projection, diffuse shading and sRGB conversion use standard numeric constants.
package reflectionbuild

import (
	"context"
	"image"
	"image/color"
	"math"

	"github.com/karty-game/karty-webgpu/pkg/reflection/scene"
)

const (
	panoramaWidth  = 512
	panoramaHeight = 256
)

// render bakes diffuse room radiance from the probe origin. It deliberately
// excludes recursive reflections and view-dependent specular highlights.
func render(ctx context.Context, authored reflectionscene.Scene) (*image.NRGBA, error) {
	pixels := image.NewNRGBA(image.Rect(0, 0, panoramaWidth, panoramaHeight))
	objects := primitives(authored.Objects)
	origin := vectorOf(authored.ProbePosition)

	for row := range panoramaHeight {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		latitude := (.5 - (float64(row)+.5)/panoramaHeight) * math.Pi
		vertical, horizontal := math.Sincos(latitude)

		for column := range panoramaWidth {
			longitude := ((float64(column)+.5)/panoramaWidth - .5) * 2 * math.Pi
			sine, cosine := math.Sincos(longitude)
			direction := vector{horizontal * cosine, vertical, horizontal * sine}

			object, normal := nearest(objects, origin, direction)
			if object >= 0 {
				pixels.SetNRGBA(column, row, shade(authored, objects[object].object.Surface, normal))
			} else {
				pixels.SetNRGBA(column, row, sky(authored, direction))
			}
		}
	}

	return pixels, nil
}

func nearest(objects []primitive, origin, direction vector) (index int, normal vector) {
	index, distance := -1, float64(reflectionRange)

	for candidate, object := range objects {
		hit, outward := object.intersect(origin, direction)
		if hit < distance {
			index, normal, distance = candidate, outward, hit
		}
	}

	return index, normal
}

func sky(authored reflectionscene.Scene, direction vector) color.NRGBA {
	blend := max(0, min(1, direction.dot(vectorOf(authored.Fill.Direction).unit())*.5+.5))
	channel := func(ground, sky float32) uint8 {
		return srgb((float64(ground)*(1-blend) + float64(sky)*blend) * float64(authored.Fill.Intensity))
	}

	return color.NRGBA{
		R: channel(authored.Fill.GroundColor.R, authored.Fill.SkyColor.R),
		G: channel(authored.Fill.GroundColor.G, authored.Fill.SkyColor.G),
		B: channel(authored.Fill.GroundColor.B, authored.Fill.SkyColor.B), A: 255,
	}
}

func shade(authored reflectionscene.Scene, surface reflectionscene.Surface, normal vector) color.NRGBA {
	blend := max(0, min(1, normal.dot(vectorOf(authored.Fill.Direction).unit())*.5+.5))
	diffuse := max(0, -normal.dot(vectorOf(authored.Key.Direction).unit())) * float64(authored.Key.Intensity)
	channel := func(base, ground, sky, key float32) uint8 {
		ambient := .02 + (float64(ground)*(1-blend)+float64(sky)*blend)*float64(authored.Fill.Intensity)

		return srgb(float64(base) * (ambient + float64(key)*diffuse))
	}

	return color.NRGBA{
		R: channel(surface.Color.R, authored.Fill.GroundColor.R, authored.Fill.SkyColor.R, authored.Key.Color.R),
		G: channel(surface.Color.G, authored.Fill.GroundColor.G, authored.Fill.SkyColor.G, authored.Key.Color.G),
		B: channel(surface.Color.B, authored.Fill.GroundColor.B, authored.Fill.SkyColor.B, authored.Key.Color.B),
		A: 255,
	}
}

func srgb(linear float64) uint8 {
	linear = max(0, min(1, linear))

	encoded := linear * 12.92
	if linear > .0031308 {
		encoded = 1.055*math.Pow(linear, 1/2.4) - .055
	}

	return uint8(math.Round(encoded * 255))
}
