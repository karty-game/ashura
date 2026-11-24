//go:build !js

//nolint:mnd // Fixed KTX2 descriptor offsets and admitted dimensions.
package assetbuild

import (
	"context"
	"encoding/binary"

	"github.com/karty-game/ashura/internal/ktx2data"
	"github.com/karty-game/ashura/internal/texturedata"
	"github.com/karty-game/ashura/pkg/texture"
)

type blockUASTCInfo struct {
	width, height, levels int
	space                 texture.ColorSpace
}

func prepareBlockUASTC(ctx context.Context, image texturedata.Data, quality int, options BlockCompressionOptions) ([]byte, error) {
	bounded, cancel, err := blockContext(ctx, options.Timeout)
	if err != nil {
		return nil, err
	}
	defer cancel()

	return preparePortableUASTC(bounded, image, quality)
}

func transcodeBlockUASTC(
	ctx context.Context,
	prepared []byte,
	info blockUASTCInfo,
	format uint32,
	options BlockCompressionOptions,
) ([][]byte, error) {
	bounded, cancel, err := blockContext(ctx, options.Timeout)
	if err != nil {
		return nil, err
	}
	defer cancel()

	return transcodeBasisBlocks(bounded, prepared, info, format)
}

func inspectBlockUASTC(data []byte) (blockUASTCInfo, error) {
	if len(data) < 80 || len(data) > 96<<20 || binary.LittleEndian.Uint32(data[12:]) != 0 {
		return blockUASTCInfo{}, errBlockCompression
	}

	descriptor := uint64(binary.LittleEndian.Uint32(data[48:]))
	if descriptor+44 > uint64(len(data)) || binary.LittleEndian.Uint32(data[52:]) != 44 || data[descriptor+12] != 166 {
		return blockUASTCInfo{}, errBlockCompression
	}
	// Numeric SSBump sources may carry unrelated alpha. Both standard UASTC
	// RGB and RGBA blocks retain the same sixteen-byte storage profile.
	channel := data[descriptor+31]
	if channel != 0 && channel != 3 {
		return blockUASTCInfo{}, errBlockCompression
	}

	format := blockVKBC7

	switch data[descriptor+14] {
	case 1:
	case 2:
		format = blockVKBC7SRGB
	default:
		return blockUASTCInfo{}, errBlockCompression
	}
	// UASTC and BC7 have identical block/sample bounds. Retag only an inspection
	// copy to reuse strict KTX admission; this copy is never published as BC7.
	inspection := append([]byte(nil), data...)
	binary.LittleEndian.PutUint32(inspection[12:], format)
	inspection[descriptor+12] = 134
	inspection[descriptor+31] = 0

	image, err := ktx2data.Decode(inspection)
	if err != nil {
		return blockUASTCInfo{}, errBlockCompression
	}

	return blockUASTCInfo{width: image.Width, height: image.Height, levels: len(image.Levels), space: image.ColorSpace}, nil
}

func preparePortableUASTC(ctx context.Context, image texturedata.Data, quality int) ([]byte, error) {
	prepared, failure := encodeBasisImage(ctx, image, quality, false)
	if failure != nil {
		return nil, failure
	}

	info, failure := inspectBlockUASTC(prepared)
	if failure != nil || info.width != image.Width || info.height != image.Height || info.levels != len(image.Levels) ||
		info.space != image.ColorSpace {
		return nil, errBlockCompression
	}

	return prepared, nil
}
