//go:build !js

package ssbump

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"math"
	"testing"
)

func TestAuthoredHeightDirectionsAndVisibility(t *testing.T) {
	t.Parallel()

	const size = 16

	source := image.NewGray(image.Rect(0, 0, size, size))
	for y := range size {
		source.SetGray(8, y, color.Gray{Y: 255})
	}

	got, err := GenerateHeight(t.Context(), source, 4, Options{Rays: 300, Distance: 4})
	if err != nil {
		t.Fatal(err)
	}

	// Two texels beside the ridge are geometrically flat, but their +U RNM
	// lobe is shadowed. A slope-only normal conversion would leave 85/85/85.
	left, right := got.NRGBAAt(6, 8), got.NRGBAAt(10, 8)
	if left.R >= 85 || left.R >= left.G || left.R >= left.B || right.R <= left.R {
		t.Fatalf("authored ridge lost directional occlusion: left=%v right=%v", left, right)
	}

	heights := make([]float64, size*size)
	for y := range size {
		for x := range size {
			heights[y*size+x] = float64(x) + 2*float64(y)
		}
	}

	normal := heightNormals(heights, size)[8*size+8]

	length := math.Sqrt(6)
	for axis, want := range [3]float64{-1 / length, -2 / length, 1 / length} {
		if math.Abs(normal[axis]-want) > 1e-12 {
			t.Fatalf("height normal axis %d = %g, want %g", axis, normal[axis], want)
		}
	}
}

func TestAuthoredHeightFlatScaleAndAdmission(t *testing.T) {
	t.Parallel()

	source := image.NewGray(image.Rect(0, 0, 8, 8))
	for index := range source.Pix {
		source.Pix[index] = 170
	}

	got, err := GenerateHeight(t.Context(), source, 8, Options{Rays: 16, Distance: 4})
	if err != nil {
		t.Fatal(err)
	}

	for index := 0; index < len(got.Pix); index += 4 {
		if !bytes.Equal(got.Pix[index:index+4], []byte{85, 85, 85, 255}) {
			t.Fatalf("flat height changed flat coefficients: %v", got.Pix[index:index+4])
		}
	}

	for _, scale := range []float64{0, -1, math.NaN(), math.Inf(1), 513} {
		if _, err := GenerateHeight(t.Context(), source, scale, Options{}); err == nil {
			t.Fatalf("accepted height scale %v", scale)
		}
	}

	colored := image.NewNRGBA(source.Bounds())
	colored.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})

	if _, err := GenerateHeight(t.Context(), colored, 8, Options{}); err == nil {
		t.Fatal("accepted a color texture as authored height")
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := GenerateHeight(ctx, source, 8, Options{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation = %v", err)
	}
}
