//go:build !js

package lightmapbuild

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/karty-game/karty-webgpu/internal/lightmapdata"
	"github.com/karty-game/karty-webgpu/pkg/lightmap"
	"github.com/karty-game/karty-webgpu/pkg/texture"
)

const denoiseChannels = 9

var errDenoise = errors.New("lightmap: invalid offline RNM denoise input or options")

// DenoiseOptions controls offline, geometry-guided filtering of indirect light.
// Zero values select Radius 2, Passes 2 and Strength 1. Strength blends each
// pass with its unfiltered input. No filtering is performed by the runtime.
type DenoiseOptions struct {
	Radius   int
	Passes   int
	Strength float32
}

// DenoiseRNM filters environment and bounced light while preserving point-light
// shadows. Direct must be a reference bake of the same asset and point lights,
// with zero environment and Bounces=0. Each input's scale is decoded separately.
// All nine RNM channels share weights; chart, world geometry and tangent frames
// prevent unrelated surfaces from mixing. Padding is regenerated after filtering.
// Inputs are immutable. Scratch uses two nine-float arrays and a surface-link
// mask (73 bytes per texel). Local visibility prevents filtering across walls.
// Returned Duration includes both supplied bake durations and denoising.
func DenoiseRNM(
	ctx context.Context,
	asset *lightmap.Asset,
	total, direct RNMResult,
	options DenoiseOptions,
) (RNMResult, error) {
	started := time.Now()

	if ctx == nil {
		return RNMResult{}, errDenoise
	}

	if err := ctx.Err(); err != nil {
		return RNMResult{}, err
	}

	options, err := normalizeDenoiseOptions(options)
	if err != nil {
		return RNMResult{}, err
	}

	document, err := denoiseDocument(asset, total, direct)
	if err != nil {
		return RNMResult{}, err
	}

	links, err := denoiseLinks(ctx, document)
	if err != nil {
		return RNMResult{}, err
	}

	current, err := denoiseDifference(ctx, document, total, direct)
	if err != nil {
		return RNMResult{}, err
	}

	next := make([]float32, len(current))
	for pass := range options.Passes {
		if err := denoisePass(ctx, document, links, current, next, options, 1<<pass); err != nil {
			return RNMResult{}, err
		}

		current, next = next, current
	}

	result, err := denoiseResolve(ctx, document, total, direct, current)
	if err != nil {
		return RNMResult{}, err
	}

	result.Duration = total.Duration + direct.Duration + time.Since(started)

	return result, nil
}

func normalizeDenoiseOptions(options DenoiseOptions) (DenoiseOptions, error) {
	if options.Radius == 0 {
		options.Radius = 2
	}

	if options.Passes == 0 {
		options.Passes = 2
	}

	if options.Strength == 0 {
		options.Strength = 1
	}

	if options.Radius < 1 || options.Radius > 4 || options.Passes < 1 || options.Passes > 4 ||
		math.IsNaN(float64(options.Strength)) || options.Strength <= 0 || options.Strength > 1 {
		return options, errDenoise
	}

	return options, nil
}

func denoiseDocument(asset *lightmap.Asset, total, direct RNMResult) (*lightmapdata.Document, error) {
	if asset == nil || asset.Document() == nil || direct.Bounces != 0 {
		return nil, errDenoise
	}

	document := asset.Document()

	pixels := int(document.Width) * int(document.Height)
	if !document.RNM() || len(document.Receivers) != pixels*lightmapdata.ReceiverStride ||
		len(document.Frames) != pixels*lightmapdata.FrameStride || len(document.Padding) != pixels*4 {
		return nil, errDenoise
	}

	for _, result := range []RNMResult{total, direct} {
		if !validDenoiseResult(result, document) {
			return nil, errDenoise
		}
	}

	return document, nil
}

func validDenoiseResult(result RNMResult, document *lightmapdata.Document) bool {
	if result.Encoding != texture.RNMRGBE || math.IsNaN(float64(result.Scale)) || math.IsInf(float64(result.Scale), 0) ||
		result.Scale <= 0 {
		return false
	}

	for _, image := range result.Textures {
		if image.Width != int(document.Width) || image.Height != int(document.Height) || image.ColorSpace != texture.Linear ||
			image.Format != texture.RGBA8 || len(image.Levels) != 1 || len(image.Levels[0]) != image.Width*image.Height*4 {
			return false
		}
	}

	return true
}
