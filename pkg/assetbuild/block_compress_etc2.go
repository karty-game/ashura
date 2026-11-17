//go:build !js

package assetbuild

import (
	"context"

	"github.com/karty-game/karty-webgpu/pkg/texture"
)

// CompressETC2RGB encodes authored opaque RGBA8 mips to native ETC2 RGB offline.
// It preserves transfer, dimensions and mip count. LDRQuality defaults to 0.
// One total deadline bounds encoding and transcoding, as with CompressBC1.
func CompressETC2RGB(ctx context.Context, source []byte, options BlockCompressionOptions) ([]byte, error) {
	bounded, cancel, err := blockContext(ctx, options.Timeout)
	if err != nil {
		return nil, err
	}
	defer cancel()

	prepared, err := PrepareOpaqueLDRUASTC(bounded, source, options)
	if err != nil {
		return nil, err
	}

	return TranscodeLDRNative(bounded, prepared, texture.ETC2RGB, options)
}

// CompressETC2RGBA encodes authored RGBA8 mip levels to native ETC2 RGBA blocks
// offline. It preserves smooth alpha, linear/sRGB transfer and authored mips.
// LDRQuality defaults to effort 2, with the same bounds as CompressBC7.
func CompressETC2RGBA(ctx context.Context, source []byte, options BlockCompressionOptions) ([]byte, error) {
	return compressAlphaLDR(ctx, source, texture.ETC2RGBA, options)
}
