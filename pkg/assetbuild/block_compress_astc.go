//go:build !js

//nolint:gosec // Decoded source dimensions are bounded to 4096 before conversion.
package assetbuild

import (
	"context"
	"encoding/binary"

	"github.com/karty-game/ashura/internal/basiscodec"
	"github.com/karty-game/ashura/internal/ktx2data"
	"github.com/karty-game/ashura/pkg/texture"
)

// CompressASTC6x6 prepares native ASTC blocks offline, preserving authored RGBA
// channels, transfer and mip dimensions. Base dimensions must be six-aligned.
// LDRQuality selects effort 0..4; zero effort is the default. Timeout bounds the
// entire operation. No decoder, repacking or mip generation runs in applications.
func CompressASTC6x6(ctx context.Context, source []byte, options BlockCompressionOptions) ([]byte, error) {
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
	if err != nil || image.Format != texture.RGBA8 || image.Width%6 != 0 || image.Height%6 != 0 {
		return nil, errBlockCompression
	}

	if image.Width > astcTileRows || image.Height > astcTileRows {
		levels, err := tiledASTC6x6(bounded, image, quality)
		if err != nil {
			return nil, err
		}

		format := blockVKASTC6x6
		if image.ColorSpace == texture.SRGB {
			format = blockVKASTC6x6SRGB
		}

		return writeCanonicalNativeBlockKTX(format, image.Width, image.Height, levels), nil
	}

	var encoded []byte

	err = useBasis(bounded, func(session *basiscodec.Session) error {
		var failure error

		encoded, failure = session.EncodeASTC6x6(bounded, basiscodec.Image{
			Width: uint32(image.Width), Height: uint32(image.Height), Levels: image.Levels, SRGB: image.ColorSpace == texture.SRGB,
		}, quality)

		return failure
	})
	if err != nil {
		return nil, err
	}

	return canonicalASTC6x6(encoded, image.Width, image.Height, len(image.Levels), image.ColorSpace)
}

func canonicalASTC6x6(encoded []byte, width, height, levels int, space texture.ColorSpace) ([]byte, error) {
	if len(encoded) < 80 || len(encoded) > maxArtifactBytes {
		return nil, errBlockCompression
	}

	descriptor := uint64(binary.LittleEndian.Uint32(encoded[48:]))
	if descriptor+44 > uint64(len(encoded)) || binary.LittleEndian.Uint32(encoded[52:]) != 44 ||
		encoded[descriptor+12] != 162 || (encoded[descriptor+31] != 0 && encoded[descriptor+31] != 3) {
		return nil, errBlockCompression
	}
	// The pinned encoder labels ASTC RGBA as channel 3. Normalize only this
	// owned build intermediate to the Khronos ASTC data-channel descriptor.
	encoded[descriptor+31] = 0

	image, err := ktx2data.Decode(encoded)
	if err != nil || image.Format != texture.ASTC6x6 || image.Width != width || image.Height != height ||
		len(image.Levels) != levels || image.ColorSpace != space {
		return nil, errBlockCompression
	}

	format := blockVKASTC6x6
	if space == texture.SRGB {
		format = blockVKASTC6x6SRGB
	}

	return writeCanonicalNativeBlockKTX(format, width, height, image.Levels), nil
}
