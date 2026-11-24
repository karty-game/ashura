//go:build !js

package assetbuild

import (
	"context"
	"errors"
	"time"

	"github.com/karty-game/ashura/internal/ktx2data"
	"github.com/karty-game/ashura/internal/texturedata"
	"github.com/karty-game/ashura/pkg/texture"
)

// BlockCompressionOptions configures offline texture encoding through embedded
// Go-hosted WASM libraries. No external encoder or compiler is used.
type BlockCompressionOptions struct {
	// SSBumpQuality selects UASTC effort 0..4; nil preserves the quality-4 profile.
	SSBumpQuality *int
	// LDRQuality selects color/lightmap and BC1/BC7 UASTC effort 0..4.
	// Nil selects 0, except CompressBC7 which defaults to 2 to bound build cost.
	LDRQuality *int
	// RGBEQuality selects mobile RGBE UASTC effort 0..4; nil preserves quality 3.
	RGBEQuality *int
	// HDREffort is the pinned Basis HDR encoder effort, from 0 (fastest) to 4.
	HDREffort int
	// Timeout bounds the complete offline operation for one texture. Zero selects 2m.
	Timeout time.Duration
}

var errBlockCompression = errors.New("invalid offline native block compression")

const (
	defaultBlockTimeout  = 2 * time.Minute
	maxBlockUASTCQuality = 4
	blockTexelDimension  = 4
)

// CompressSSBumpBC7 preserves an authored linear coefficient mip chain, encoding
// it to GPU-native BC7. These RGB coefficients are not tangent normal vectors;
// no normal-map transformation, mip generation or resizing is performed.
func CompressSSBumpBC7(ctx context.Context, source []byte, options BlockCompressionOptions) ([]byte, error) {
	return compressSSBumpNative(ctx, source, texture.BC7, options)
}

func blockInput(source []byte) (texturedata.Data, error) {
	image, err := ktx2data.Decode(source)
	if err != nil || image.ColorSpace != texturedata.Linear || image.Format != texturedata.RGBA8 || image.Width < 4 ||
		image.Height < 4 || image.Width%4 != 0 || image.Height%4 != 0 {
		return texturedata.Data{}, errBlockCompression
	}

	return image, nil
}

func blockContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc, error) {
	if ctx == nil || timeout < 0 {
		return nil, nil, errBlockCompression
	}

	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}

	if timeout == 0 {
		timeout = defaultBlockTimeout
	}

	bounded, cancel := context.WithTimeout(ctx, timeout)

	return bounded, cancel, nil
}

func blockUASTCQuality(selected *int, fallback int) (int, error) {
	if selected == nil {
		return fallback, nil
	}

	if *selected < 0 || *selected > maxBlockUASTCQuality {
		return 0, errBlockCompression
	}

	return *selected, nil
}
