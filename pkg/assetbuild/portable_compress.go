//go:build !js

package assetbuild

import (
	"context"

	"github.com/karty-game/karty-webgpu/internal/basistranscode"
	"github.com/karty-game/karty-webgpu/internal/texturedata"
	"github.com/karty-game/karty-webgpu/pkg/texture"
)

const (
	portableRNMQuality    = 3
	portableSSBumpQuality = 4
)

// CompressRGBEETC2 preserves HDR scale with a shared exponent per 4x4 block.
// All conversion runs offline. Native EAC alpha is exact; the existing RGBE
// shader decodes the resulting ETC2RGBA texture without a new runtime codec.
func CompressRGBEETC2(ctx context.Context, source []byte, options BlockCompressionOptions) ([]byte, error) {
	quality, err := blockUASTCQuality(options.RGBEQuality, portableRNMQuality)
	if err != nil {
		return nil, err
	}

	image, err := blockInput(source)
	if err != nil || len(image.Levels) != 1 || ctx == nil {
		return nil, errBlockCompression
	}

	if err = ctx.Err(); err != nil {
		return nil, err
	}

	mantissas, exponents := portableRGBEBlocks(image)
	image.Levels = [][]byte{mantissas}

	levels, err := portableBlockColor(ctx, image, options, blockVKETC2RGB, quality)
	if err != nil {
		return nil, err
	}

	blocks := portableRGBAColorBlocks(levels[0], exponents)

	return writeNativeBlockKTX(blockVKETC2RGBA, image.Width, image.Height, [][]byte{blocks}), nil
}

// CompressSSBumpETC2 encodes numeric coefficient mips to native ETC2RGB offline.
// No normal-map conversion, resizing or mip generation is performed.
func CompressSSBumpETC2(ctx context.Context, source []byte, options BlockCompressionOptions) ([]byte, error) {
	quality, err := blockUASTCQuality(options.SSBumpQuality, portableSSBumpQuality)
	if err != nil {
		return nil, err
	}

	image, err := blockInput(source)
	if err != nil {
		return nil, errBlockCompression
	}

	levels, err := portableBlockColor(ctx, image, options, blockVKETC2RGB, quality)
	if err != nil {
		return nil, err
	}

	return writeNativeBlockKTX(blockVKETC2RGB, image.Width, image.Height, levels), nil
}

// CompressSSBumpASTC preserves independent numeric coefficient mip chains as
// native ASTC4x4 blocks offline. It does not use normal-map transforms.
func CompressSSBumpASTC(ctx context.Context, source []byte, options BlockCompressionOptions) ([]byte, error) {
	return compressSSBumpNative(ctx, source, texture.ASTC4x4, options)
}

// TranscodeAlbedoNative converts already prepared ETC1S to a selected native
// sRGB BC1 or ETC2RGB mip chain offline, removing the runtime decoder dependency.
func TranscodeAlbedoNative(ctx context.Context, source []byte, target texture.Format, options BlockCompressionOptions) ([]byte, error) {
	info, err := basistranscode.Inspect(source)
	if err != nil {
		return nil, err
	}

	format := blockVKBC1SRGB
	if target == texture.ETC2RGB {
		format = blockVKETC2RGBSRGB
	} else if target != texture.BC1 {
		return nil, errBlockCompression
	}

	bounded, cancel, err := blockContext(ctx, options.Timeout)
	if err != nil {
		return nil, err
	}
	defer cancel()

	levels, err := transcodeBasisBlocks(
		bounded,
		source,
		blockUASTCInfo{width: info.Width, height: info.Height, levels: info.Levels, space: texture.SRGB},
		format,
	)
	if err != nil {
		return nil, err
	}

	return writeNativeBlockKTX(format, info.Width, info.Height, levels), nil
}

func portableBlockColor(
	ctx context.Context,
	image texturedata.Data,
	options BlockCompressionOptions,
	format uint32,
	quality int,
) ([][]byte, error) {
	bounded, cancel, err := blockContext(ctx, options.Timeout)
	if err != nil {
		return nil, err
	}
	defer cancel()

	prepared, err := encodeBasisImage(bounded, image, quality, false)
	if err != nil {
		return nil, err
	}

	return transcodeBasisBlocks(
		bounded,
		prepared,
		blockUASTCInfo{width: image.Width, height: image.Height, levels: len(image.Levels), space: image.ColorSpace},
		format,
	)
}

// Constant EAC alpha uses base=exponent, multiplier=0 and zero selectors.
// Only the base contributes, preserving every stored RGBE exponent exactly.
func portableRGBAColorBlocks(colors, exponents []byte) []byte {
	blocks := make([]byte, len(exponents)*16)
	for index, exponent := range exponents {
		blocks[index*16] = exponent
		copy(blocks[index*16+8:], colors[index*8:index*8+8])
	}

	return blocks
}
