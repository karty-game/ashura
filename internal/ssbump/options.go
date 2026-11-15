//go:build !js

package ssbump

import (
	"errors"
	"fmt"
	"math"
)

const (
	// DefaultRays preserves the original offline sample's hemisphere integration.
	DefaultRays = 300
	// DefaultDistance is the periodic visibility reach in source texels.
	DefaultDistance = 32
	// DefaultBias offsets visibility origins above integrated surface relief.
	DefaultBias             = .05
	maximumRays             = 4096
	maximumDistance         = 256
	maximumBias             = 32
	maximumDimension        = 512
	maximumRaySteps  uint64 = 1 << 31
)

var errOptions = errors.New("invalid SSBump bake options or source dimensions")

// Options bounds the deterministic visibility integrator. Zero fields select
// the original defaults; positive values explicitly override them.
type Options struct {
	Rays     int
	Distance float64
	Bias     float64
}

// NormalizeOptions applies defaults and rejects unsupported or non-finite work.
func NormalizeOptions(options Options) (Options, error) {
	if options.Rays == 0 {
		options.Rays = DefaultRays
	}

	if options.Distance == 0 {
		options.Distance = DefaultDistance
	}

	if options.Bias == 0 {
		options.Bias = DefaultBias
	}

	if options.Rays < 1 || options.Rays > maximumRays ||
		!finite(options.Distance) || options.Distance < .5 || options.Distance > maximumDistance ||
		!finite(options.Bias) || options.Bias <= 0 || options.Bias > maximumBias {
		return Options{}, errOptions
	}

	return options, nil
}

func finite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }

func validateSize(width, height int, options Options) error {
	if width < 2 || width > maximumDimension || width != height || width&(width-1) != 0 {
		return fmt.Errorf("SSBump normals must be square power-of-two images between 2 and %d pixels: %w", maximumDimension, errOptions)
	}

	//nolint:gosec // Dimensions and normalized settings are positive and bounded before these work-budget conversions.
	work := uint64(width) * uint64(height) * uint64(options.Rays) * uint64(math.Ceil(options.Distance))
	if work > maximumRaySteps {
		return fmt.Errorf("SSBump bake exceeds %d bounded ray steps: %w", maximumRaySteps, errOptions)
	}

	return nil
}
