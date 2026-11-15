//go:build !js

//nolint:varnamelen // Coordinate and index names match the analytical heightfield fixture.
package ssbump

import (
	"image"
	"image/color"
	"math"
	"testing"
)

func TestPeriodicNormalIntegration(t *testing.T) {
	t.Parallel()

	const size = 16

	expected := make([]float64, size*size)
	for y := range size {
		for x := range size {
			expected[y*size+x] = 2*math.Sin(2*math.Pi*float64(x)/size) + math.Cos(2*math.Pi*float64(y)/size)
		}
	}

	normals := make([][3]float64, len(expected))

	for y := range size {
		for x := range size {
			i := y*size + x
			dx := expected[y*size+(x+1)%size] - expected[i]
			dy := expected[((y+1)%size)*size+x] - expected[i]
			n := [3]float64{-dx, -dy, 1}

			length := math.Sqrt(dot(n, n))
			for c := range 3 {
				n[c] /= length
			}

			normals[i] = n
		}
	}

	actual := integrate(normals, size)
	for i, want := range expected {
		if math.Abs(actual[i]-want) > 1e-10 {
			t.Fatalf("height %d = %g, want %g", i, actual[i], want)
		}
	}
}

func TestFlatUnoccludedCoefficients(t *testing.T) {
	t.Parallel()

	const size = 16

	options := testOptions(t)

	got := coefficient([3]float64{0, 0, 1}, make([]float64, size*size), size, 0, 0, directions(options.Rays), options)
	if got != (color.NRGBA{R: 85, G: 85, B: 85, A: 255}) {
		t.Fatalf("flat coefficients = %v", got)
	}
}

func TestDirectionalVisibility(t *testing.T) {
	t.Parallel()

	const size = 32

	heights := make([]float64, size*size)
	for y := range size {
		heights[y*size+8] = 4
	}

	toward := ray{dx: 1, slope: .25}
	away := ray{dx: -1, slope: .25}

	options := testOptions(t)
	if unoccluded(heights, size, 6, 16, toward, options) {
		t.Fatal("ray passed through raised relief")
	}

	if !unoccluded(heights, size, 6, 16, away, options) {
		t.Fatal("open direction was incorrectly blocked")
	}

	got := coefficient([3]float64{0, 0, 1}, heights, size, 6, 16, directions(options.Rays), options)
	if got.R >= 85 || got.R >= got.G || got.R >= got.B {
		t.Fatalf("wall along +U did not selectively occlude +U lobe: %v", got)
	}
}

func TestBakeFlatNormalMap(t *testing.T) {
	t.Parallel()

	normal := image.NewNRGBA(image.Rect(0, 0, 8, 8))

	for y := range 8 {
		for x := range 8 {
			normal.SetNRGBA(x, y, color.NRGBA{R: 128, G: 128, B: 255, A: 255})
		}
	}

	got, err := Generate(t.Context(), normal, Options{})
	if err != nil {
		t.Fatal(err)
	}

	for y := range 8 {
		for x := range 8 {
			p := got.NRGBAAt(x, y)
			if p.A != 255 || p.R < 83 || p.R > 87 || p.G < 83 || p.G > 87 || p.B < 83 || p.B > 87 {
				t.Fatalf("flat pixel %d,%d = %v", x, y, p)
			}
		}
	}
}

func testOptions(t *testing.T) Options {
	t.Helper()

	options, err := NormalizeOptions(Options{})
	if err != nil {
		t.Fatal(err)
	}

	return options
}
