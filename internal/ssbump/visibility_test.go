//go:build !js

//nolint:varnamelen // Scalar oracle deliberately retains the original visibility arithmetic.
package ssbump

import (
	"image/color"
	"math"
	"testing"
)

func coefficient(normal [3]float64, heights []float64, size, x, y int, rays []ray, options Options) color.NRGBA {
	for index := range rays {
		r := &rays[index]
		r.steps = prepareRaySteps(*r, options)
	}

	return preparedCoefficient(normal, heights, size, x, y, rays, options.Bias, heightCeiling(heights))
}

func unoccluded(heights []float64, size, x, y int, r ray, options Options) bool {
	start := heights[y*size+x] + options.Bias
	for distance := .5; distance <= options.Distance; distance++ {
		if height(heights, size, float64(x)+r.dx*distance, float64(y)+r.dy*distance) > start+r.slope*distance {
			return false
		}
	}

	return true
}

func TestPreparedVisibilityMatchesScalar(t *testing.T) {
	t.Parallel()

	const size = 32

	options := Options{Rays: 128, Distance: 16, Bias: .05}

	rays := directions(options.Rays)
	for index := range rays {
		r := &rays[index]
		r.steps = prepareRaySteps(*r, options)
	}

	for _, scale := range []float64{0, 1e-100, 1, 512} {
		heights := make([]float64, size*size)
		for i := range heights {
			heights[i] = scale * (math.Sin(float64(i)*1.618) + math.Cos(float64(i)*.37))
		}

		ceiling := heightCeiling(heights)

		for y := range size {
			for x := range size {
				for index, r := range rays {
					got := preparedUnoccluded(heights, size, x, y, r.steps, options.Bias, ceiling)

					want := unoccluded(heights, size, x, y, r, options)
					if got != want {
						t.Fatalf("scale %g pixel %d,%d ray %d: %t != %t", scale, x, y, index, got, want)
					}
				}
			}
		}
	}
}

func height(heights []float64, size int, x, y float64) float64 {
	fx, fy := math.Floor(x), math.Floor(y)
	xx, yy := int(fx)&(size-1), int(fy)&(size-1)
	nextX, nextY := (xx+1)&(size-1), (yy+1)&(size-1)
	tx, ty := x-fx, y-fy
	lower := heights[yy*size+xx]*(1-tx) + heights[yy*size+nextX]*tx
	upper := heights[nextY*size+xx]*(1-tx) + heights[nextY*size+nextX]*tx

	return lower*(1-ty) + upper*ty
}
