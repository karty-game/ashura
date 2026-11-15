//go:build !js

//nolint:mnd,varnamelen // Heights are linear unsigned samples; central differences use source-texel units.
package ssbump

import (
	"context"
	"fmt"
	"image"
	"math"
)

// GenerateHeight uses authored relief directly with the same RNM projection
// and visibility integrator as Generate. No normal integration or intermediate
// normal-map quantization is needed. Scale is the full height range in texels.
func GenerateHeight(ctx context.Context, source image.Image, scale float64, options Options) (*image.NRGBA, error) {
	options, err := NormalizeOptions(options)
	if err != nil {
		return nil, err
	}

	size := source.Bounds().Dx()
	if err := validateSize(size, source.Bounds().Dy(), options); err != nil {
		return nil, err
	}

	if !finite(scale) || scale <= 0 || scale > maximumDimension {
		return nil, errOptions
	}

	heights := make([]float64, size*size)
	for y := range size {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		for x := range size {
			r, g, b, _ := source.At(source.Bounds().Min.X+x, source.Bounds().Min.Y+y).RGBA()
			if r != g || r != b {
				return nil, fmt.Errorf("authored height must be grayscale: %w", errOptions)
			}

			heights[y*size+x] = float64(r) / 65535 * scale
		}
	}

	normals := heightNormals(heights, size)

	result := image.NewNRGBA(image.Rect(0, 0, size, size))
	if err := render(ctx, normals, heights, directions(options.Rays), result, options); err != nil {
		return nil, err
	}

	return result, nil
}

func heightNormals(heights []float64, size int) [][3]float64 {
	normals := make([][3]float64, len(heights))

	for y := range size {
		for x := range size {
			dx := (heights[y*size+((x+1)&(size-1))] - heights[y*size+((x-1)&(size-1))]) * .5
			dy := (heights[((y+1)&(size-1))*size+x] - heights[((y-1)&(size-1))*size+x]) * .5
			normal := [3]float64{-dx, -dy, 1}

			length := math.Sqrt(dot(normal, normal))
			for c := range normal {
				normal[c] /= length
			}

			normals[y*size+x] = normal
		}
	}

	return normals
}
