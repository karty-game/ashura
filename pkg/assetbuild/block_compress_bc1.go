//go:build !js

package assetbuild

import (
	"bytes"
	"context"

	"github.com/karty-game/ashura/internal/basiscodec"
	"github.com/karty-game/ashura/internal/ktx2data"
	"github.com/karty-game/ashura/internal/texturedata"
)

// CompressBC1 encodes prepared RGBA8 mip levels to GPU-ready BC1 RGBA KTX2.
// Alpha is exactly one bit: source alpha >=128 is opaque. Embedded WASM chooses
// color endpoints; Go applies the authored mask offline. Linear/sRGB transfer,
// dimensions and authored mips are retained. LDRQuality selects effort 0..4.
// No native encoder, runtime conversion, resizing or mip generation is used.
func CompressBC1(ctx context.Context, source []byte, options BlockCompressionOptions) ([]byte, error) {
	bounded, cancel, err := blockContext(ctx, options.Timeout)
	if err != nil {
		return nil, err
	}
	defer cancel()

	quality, err := blockUASTCQuality(options.LDRQuality, 0)
	if err != nil {
		return nil, err
	}

	image, err := ktx2data.Decode(source)
	if err != nil || image.Format != texturedata.RGBA8 || image.Width%blockTexelDimension != 0 ||
		image.Height%blockTexelDimension != 0 {
		return nil, errBlockCompression
	}

	opaque, err := bc1OpaqueLevels(bounded, image)
	if err != nil {
		return nil, err
	}

	var result basiscodec.Result

	err = useBasis(bounded, func(session *basiscodec.Session) error {
		prepared, failure := session.Encode(bounded, basiscodec.Image{
			//nolint:gosec // KTX2 admission bounds both dimensions to 1..4096.
			Width: uint32(image.Width), Height: uint32(image.Height), Levels: opaque, SRGB: image.ColorSpace == texturedata.SRGB,
		}, quality, false)
		if failure != nil {
			return failure
		}

		result, failure = session.Transcode(bounded, prepared, 2)

		return failure
	})
	if err != nil {
		return nil, err
	}

	return bc1MaskedContainer(bounded, image, result)
}

func bc1OpaqueLevels(ctx context.Context, image texturedata.Data) ([][]byte, error) {
	levels := make([][]byte, len(image.Levels))
	for level, pixels := range image.Levels {
		levels[level] = bytes.Clone(pixels)

		width, height := max(1, image.Width>>level), max(1, image.Height>>level)
		for row := range height {
			if err := ctx.Err(); err != nil {
				return nil, err
			}

			for column := range width {
				levels[level][(row*width+column)*imageChannels+imageChannels-1] = byteMaximum
			}
		}
	}

	return levels, nil
}

func bc1MaskedContainer(ctx context.Context, image texturedata.Data, result basiscodec.Result) ([]byte, error) {
	if int(result.Width) != image.Width || int(result.Height) != image.Height || len(result.Levels) != len(image.Levels) ||
		result.SRGB != (image.ColorSpace == texturedata.SRGB) {
		return nil, errBlockCompression
	}

	for level, blocks := range result.Levels {
		if err := patchBC1Level(ctx, max(1, image.Width>>level), max(1, image.Height>>level), image.Levels[level], blocks); err != nil {
			return nil, err
		}
	}

	format := blockVKBC1RGBA
	if result.SRGB {
		format = blockVKBC1RGBASRGB
	}

	return writeCanonicalNativeBlockKTX(format, image.Width, image.Height, result.Levels), nil
}
