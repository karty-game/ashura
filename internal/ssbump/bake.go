//go:build !js

// Package ssbump prepares directional visibility coefficients offline. It is
// never imported by the sample application or either rendering backend.
//
//nolint:mnd,varnamelen // RNM constants, normalized byte ranges and coordinate indices describe offline numerical integration.
package ssbump

import (
	"context"
	"image"
	"image/color"
	"math"
	"runtime"
	"sync"
)

//nolint:gochecknoglobals // Immutable RNM basis shared by the offline visibility integrator.
var basis = [3][3]float64{
	{math.Sqrt(2.0 / 3), 0, 1 / math.Sqrt(3)},
	{-1 / math.Sqrt(6), 1 / math.Sqrt(2), 1 / math.Sqrt(3)},
	{-1 / math.Sqrt(6), -1 / math.Sqrt(2), 1 / math.Sqrt(3)},
}

type ray struct {
	direction     [3]float64
	dx, dy, slope float64
	lobe          [3]float64
	steps         []visibilityStep
}

type visibilityStep struct {
	x, y       int
	u, v, rise float64
}

func prepareRaySteps(r ray, options Options) []visibilityStep {
	steps := make([]visibilityStep, 0, int(math.Ceil(options.Distance)))
	for distance := .5; distance <= options.Distance; distance++ {
		dx, dy := r.dx*distance, r.dy*distance
		x, y := math.Floor(dx), math.Floor(dy)
		steps = append(steps, visibilityStep{int(x), int(y), dx - x, dy - y, r.slope * distance})
	}

	return steps
}

// Generate preserves the supplied normal directions, projects them into Valve's
// squared RNM weights and multiplies by independently integrated lobe visibility.
// The periodic heightfield is a least-squares integration of the input normals;
// it estimates relief for this demonstration, not measured surface geometry.
func Generate(ctx context.Context, normal image.Image, options Options) (*image.NRGBA, error) {
	options, err := NormalizeOptions(options)
	if err != nil {
		return nil, err
	}

	size := normal.Bounds().Dx()
	if err := validateSize(size, normal.Bounds().Dy(), options); err != nil {
		return nil, err
	}

	normals, err := normalDirections(ctx, normal, size)
	if err != nil {
		return nil, err
	}

	heights := integrate(normals, size)

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	rays := directions(options.Rays)

	result := image.NewNRGBA(image.Rect(0, 0, size, size))
	if err := render(ctx, normals, heights, rays, result, options); err != nil {
		return nil, err
	}

	return result, nil
}

func normalDirections(ctx context.Context, normal image.Image, size int) ([][3]float64, error) {
	normals := make([][3]float64, size*size)
	for y := range size {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		for x := range size {
			r, g, b, _ := normal.At(normal.Bounds().Min.X+x, normal.Bounds().Min.Y+y).RGBA()
			n := [3]float64{float64(r)/32767.5 - 1, float64(g)/32767.5 - 1, float64(b)/32767.5 - 1}

			length := math.Sqrt(dot(n, n))
			for c := range 3 {
				n[c] /= length
			}

			normals[y*size+x] = n
		}
	}

	return normals, nil
}

func render(ctx context.Context, normals [][3]float64, heights []float64, rays []ray, result *image.NRGBA, options Options) error {
	size := result.Bounds().Dx()
	ceiling := heightCeiling(heights)

	for index := range rays {
		r := &rays[index]
		r.steps = prepareRaySteps(*r, options)
	}

	var workers sync.WaitGroup

	count := min(runtime.GOMAXPROCS(0), size)
	for worker := range count {
		workers.Go(func() {
			for y := worker; y < size; y += count {
				for x := range size {
					if ctx.Err() != nil {
						return
					}

					result.SetNRGBA(x, y, preparedCoefficient(normals[y*size+x], heights, size, x, y, rays, options.Bias, ceiling))
				}
			}
		})
	}

	workers.Wait()

	return ctx.Err()
}

func directions(count int) []ray {
	rays := make([]ray, count)

	const goldenAngle = 2.399963229728653

	for i := range rays {
		z := (float64(i) + .5) / float64(count)
		azimuth := float64(i) * goldenAngle
		horizontal := math.Sqrt(1 - z*z)

		r := ray{
			direction: [3]float64{math.Cos(azimuth) * horizontal, math.Sin(azimuth) * horizontal, z},
			dx:        math.Cos(azimuth),
			dy:        math.Sin(azimuth),
			slope:     z / horizontal,
		}
		for c := range 3 {
			r.lobe[c] = math.Max(0, dot(r.direction, basis[c]))
		}

		rays[i] = r
	}

	return rays
}

func preparedCoefficient(normal [3]float64, heights []float64, size, x, y int, rays []ray, bias, ceiling float64) color.NRGBA {
	var (
		weights, total, visible [3]float64
		sum                     float64
	)

	for c := range 3 {
		p := math.Max(0, dot(normal, basis[c]))
		weights[c] = p * p
		sum += weights[c]
	}

	for _, r := range rays {
		cosine := math.Max(0, dot(normal, r.direction))
		if cosine == 0 {
			continue
		}

		open := preparedUnoccluded(heights, size, x, y, r.steps, bias, ceiling)
		for c := range 3 {
			contribution := cosine * r.lobe[c]

			total[c] += contribution
			if open {
				visible[c] += contribution
			}
		}
	}

	var channels [3]uint8

	for c := range 3 {
		value := 0.0
		if total[c] > 0 && sum > 0 {
			value = weights[c] / sum * visible[c] / total[c]
		}

		channels[c] = uint8(math.Round(255 * math.Max(0, math.Min(1, value))))
	}

	return color.NRGBA{R: channels[0], G: channels[1], B: channels[2], A: 255}
}

// A bilinear sample cannot exceed the greatest source height, apart from
// rounding of the three convex combinations. Four outward ulps cover those
// roundings even at exponent boundaries. Once an ascending ray reaches this
// ceiling, every remaining step is necessarily visible.
func heightCeiling(heights []float64) float64 {
	ceiling := heights[0]
	for _, value := range heights[1:] {
		ceiling = math.Max(ceiling, value)
	}

	for range 4 {
		ceiling = math.Nextafter(ceiling, math.Inf(1))
	}

	return ceiling
}

func preparedUnoccluded(heights []float64, size, x, y int, steps []visibilityStep, bias, ceiling float64) bool {
	start := heights[y*size+x] + bias
	for _, step := range steps {
		limit := start + step.rise
		if limit >= ceiling {
			return true
		}

		if preparedHeight(heights, size, x, y, step) > limit {
			return false
		}
	}

	return true
}

// Integer texel translations share the ray's bilinear footprint. Factoring
// the translation avoids floor/multiply work at every pixel and step. The
// fractional weights retain float64; only last-bit addition rounding differs
// from forming an absolute floating-point coordinate at every pixel.
func preparedHeight(heights []float64, size, x, y int, step visibilityStep) float64 {
	xx, yy := (x+step.x)&(size-1), (y+step.y)&(size-1)
	nextX, nextY := (xx+1)&(size-1), (yy+1)&(size-1)
	lower := heights[yy*size+xx]*(1-step.u) + heights[yy*size+nextX]*step.u
	upper := heights[nextY*size+xx]*(1-step.u) + heights[nextY*size+nextX]*step.u

	return lower*(1-step.v) + upper*step.v
}

func dot(a, b [3]float64) float64 { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }
