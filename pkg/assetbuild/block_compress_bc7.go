//go:build !js

package assetbuild

import (
	"context"

	"github.com/karty-game/ashura/internal/ktx2data"
	"github.com/karty-game/ashura/internal/texturedata"
	"github.com/karty-game/ashura/pkg/texture"
)

// CompressBC7 encodes authored RGBA8 mip levels to GPU-ready BC7 KTX2 offline.
// It retains all four channels, linear/sRGB transfer, dimensions and mip count.
// LDRQuality selects effort 0..4; nil selects 2 to bound offline preparation cost.
// No normal-map transform, resizing, mip generation or runtime decoder is used.
func CompressBC7(ctx context.Context, source []byte, options BlockCompressionOptions) ([]byte, error) {
	return compressAlphaLDR(ctx, source, texture.BC7, options)
}

func compressAlphaLDR(ctx context.Context, source []byte, target texture.Format, options BlockCompressionOptions) ([]byte, error) {
	bounded, cancel, err := blockContext(ctx, options.Timeout)
	if err != nil {
		return nil, err
	}
	defer cancel()

	const defaultBC7Quality = 2

	quality, err := blockUASTCQuality(options.LDRQuality, defaultBC7Quality)
	if err != nil {
		return nil, err
	}

	image, err := ktx2data.Decode(source)
	if err != nil || image.Format != texturedata.RGBA8 || image.Width%blockTexelDimension != 0 ||
		image.Height%blockTexelDimension != 0 {
		return nil, errBlockCompression
	}

	prepared, err := preparePortableUASTC(bounded, image, quality)
	if err != nil {
		return nil, err
	}

	return TranscodeLDRNative(bounded, prepared, target, options)
}
