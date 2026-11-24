//go:build !js

package assetbuild

import (
	"context"

	"github.com/karty-game/ashura/pkg/texture"
)

// PrepareSSBumpUASTC encodes authored linear coefficient mips at the explicit
// SSBumpQuality (quality 4 by default), shared by native texture exports;
// it is a build artifact, not a runtime texture or a normal-map transform.
func PrepareSSBumpUASTC(ctx context.Context, source []byte, options BlockCompressionOptions) ([]byte, error) {
	quality, err := blockUASTCQuality(options.SSBumpQuality, portableSSBumpQuality)
	if err != nil {
		return nil, err
	}

	image, err := blockInput(source)
	if err != nil {
		return nil, errBlockCompression
	}

	return prepareBlockUASTC(ctx, image, quality, options)
}

// TranscodeSSBumpNative converts prepared linear UASTC coefficients into native
// BC7, ASTC4x4 or ETC2RGB offline, retaining dimensions and authored mips.
func TranscodeSSBumpNative(ctx context.Context, prepared []byte, target texture.Format, options BlockCompressionOptions) ([]byte, error) {
	info, err := inspectBlockUASTC(prepared)
	if err != nil || info.space != texture.Linear {
		return nil, errBlockCompression
	}

	format := blockVKBC7

	switch target {
	case texture.BC7:
	case texture.ASTC4x4:
		format = blockVKASTC4x4
	case texture.ETC2RGB:
		format = blockVKETC2RGB
	case texture.RGBA8, texture.BC1, texture.BC6H, texture.ETC2RGBA, texture.ASTC6x6:
		return nil, errBlockCompression
	default:
		return nil, errBlockCompression
	}

	levels, err := transcodeBlockUASTC(ctx, prepared, info, format, options)
	if err != nil {
		return nil, err
	}

	return writeNativeBlockKTX(format, info.width, info.height, levels), nil
}

func compressSSBumpNative(ctx context.Context, source []byte, target texture.Format, options BlockCompressionOptions) ([]byte, error) {
	// One total deadline covers both offline codec stages.
	bounded, cancel, err := blockContext(ctx, options.Timeout)
	if err != nil {
		return nil, err
	}
	defer cancel()

	prepared, err := PrepareSSBumpUASTC(bounded, source, options)
	if err != nil {
		return nil, err
	}

	return TranscodeSSBumpNative(bounded, prepared, target, options)
}

func inspectSSBumpUASTC(data []byte) (int, int, int, error) {
	info, err := inspectBlockUASTC(data)
	if err != nil || info.space != texture.Linear {
		return 0, 0, 0, errBlockCompression
	}

	return info.width, info.height, info.levels, nil
}
