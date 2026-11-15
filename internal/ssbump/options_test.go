//go:build !js

package ssbump

import (
	"context"
	"errors"
	"image"
	"math"
	"testing"
)

func TestOptionsAndSourceBounds(t *testing.T) {
	t.Parallel()

	defaults := testOptions(t)
	if defaults.Rays != 300 || defaults.Distance != 32 || defaults.Bias != .05 {
		t.Fatalf("original baker defaults changed: %+v", defaults)
	}

	for name, options := range map[string]Options{
		"negative-rays":     {Rays: -1},
		"excess-rays":       {Rays: 4097},
		"negative-distance": {Distance: -1},
		"short-distance":    {Distance: .1},
		"excess-distance":   {Distance: 257},
		"nan-distance":      {Distance: math.NaN()},
		"infinite-distance": {Distance: math.Inf(1)},
		"negative-bias":     {Bias: -.1},
		"excess-bias":       {Bias: 33},
		"nan-bias":          {Bias: math.NaN()},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if _, err := NormalizeOptions(options); err == nil {
				t.Fatal("accepted invalid bake options")
			}
		})
	}

	for _, size := range [][2]int{{1, 1}, {3, 3}, {4, 2}, {1024, 1024}, {512, 512}} {
		if err := validateSize(size[0], size[1], defaults); err == nil {
			t.Fatalf("accepted invalid dimensions or work budget: %v", size)
		}
	}

	if err := validateSize(512, 512, Options{Rays: 8, Distance: 1, Bias: .05}); err != nil {
		t.Fatalf("bounded large-source bake rejected: %v", err)
	}
}

func TestGenerateCanceledBeforeWork(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if result, err := Generate(ctx, image.NewNRGBA(image.Rect(0, 0, 8, 8)), Options{}); !errors.Is(err, context.Canceled) || result != nil {
		t.Fatalf("canceled bake returned %v %v", result, err)
	}
}
