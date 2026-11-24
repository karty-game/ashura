//go:build !js

package assetbuild

import (
	"context"

	"github.com/karty-game/ashura/internal/basistranscode"
	"github.com/karty-game/ashura/internal/ktx2data"
	"github.com/karty-game/ashura/internal/texturedata"
	"github.com/karty-game/ashura/pkg/texture"
)

// PrepareOpaqueLDRUASTC prepares a shared offline UASTC intermediate from opaque
// RGBA8 in linear or sRGB transfer. LDRQuality defaults to effort 0. Authored
// base dimensions must be four-texel aligned. Authored smaller mips are retained;
// no alpha dropping, resizing or mip generation occurs.
func PrepareOpaqueLDRUASTC(ctx context.Context, source []byte, options BlockCompressionOptions) ([]byte, error) {
	quality, err := blockUASTCQuality(options.LDRQuality, 0)
	if err != nil {
		return nil, err
	}

	image, err := opaqueLDRBlockInput(source)
	if err != nil {
		return nil, err
	}

	return prepareBlockUASTC(ctx, image, quality, options)
}

// TranscodeLDRNative exports LDR UASTC or bounded opaque sRGB ETC1S to native BC1,
// BC7, ETC2RGB or ETC2RGBA, preserving its transfer and authored mip chain.
// RGB targets discard unused numeric alpha; color inputs should use the opaque
// preparer. Applications load native blocks without a Basis decoder.
func TranscodeLDRNative(ctx context.Context, prepared []byte, target texture.Format, options BlockCompressionOptions) ([]byte, error) {
	info, err := inspectPreparedLDR(prepared)
	if err != nil {
		return nil, err
	}

	format, err := ldrNativeTarget(target, info.space)
	if err != nil {
		return nil, err
	}

	levels, err := transcodeBlockUASTC(ctx, prepared, info, format, options)
	if err != nil {
		return nil, err
	}

	return writeCanonicalNativeBlockKTX(format, info.width, info.height, levels), nil
}

func opaqueLDRBlockInput(source []byte) (texturedata.Data, error) {
	image, err := ktx2data.Decode(source)
	if err != nil || image.Format != texture.RGBA8 || (image.ColorSpace != texture.Linear && image.ColorSpace != texture.SRGB) ||
		image.Width%blockTexelDimension != 0 || image.Height%blockTexelDimension != 0 {
		return texturedata.Data{}, errBlockCompression
	}

	for _, level := range image.Levels {
		for offset := imageChannels - 1; offset < len(level); offset += imageChannels {
			if level[offset] != byteMaximum {
				return texturedata.Data{}, errBlockCompression
			}
		}
	}

	return image, nil
}

func ldrNativeTarget(target texture.Format, space texture.ColorSpace) (uint32, error) {
	if space != texture.Linear && space != texture.SRGB {
		return 0, errBlockCompression
	}

	format := blockVKBC1

	switch target {
	case texture.BC1:
		if space == texture.SRGB {
			format = blockVKBC1SRGB
		}
	case texture.BC7:
		format = blockVKBC7
		if space == texture.SRGB {
			format = blockVKBC7SRGB
		}
	case texture.ETC2RGB:
		format = blockVKETC2RGB
		if space == texture.SRGB {
			format = blockVKETC2RGBSRGB
		}
	case texture.ETC2RGBA:
		format = blockVKETC2RGBA
		if space == texture.SRGB {
			format = blockVKETC2RGBASRGB
		}
	case texture.RGBA8, texture.BC6H, texture.ASTC4x4, texture.ASTC6x6:
		return 0, errBlockCompression
	default:
		return 0, errBlockCompression
	}

	return format, nil
}

func inspectPreparedLDR(data []byte) (blockUASTCInfo, error) {
	if info, err := inspectBlockUASTC(data); err == nil {
		return info, nil
	}

	info, err := basistranscode.Inspect(data)
	if err != nil {
		return blockUASTCInfo{}, errBlockCompression
	}

	return blockUASTCInfo{width: info.Width, height: info.Height, levels: info.Levels, space: texture.SRGB}, nil
}
