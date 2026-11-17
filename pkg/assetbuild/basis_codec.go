//go:build !js

//nolint:gosec // Image dimensions are admitted by the bounded KTX2 parser before wasm32 conversion.
package assetbuild

import (
	"context"
	"encoding/binary"
	"fmt"

	"github.com/karty-game/karty-webgpu/internal/basiscodec"
	"github.com/karty-game/karty-webgpu/internal/basistranscode"
	"github.com/karty-game/karty-webgpu/internal/texturedata"
)

type basisSessionKey struct{}

func buildCodecContext(ctx context.Context) (context.Context, func() error) {
	if ctx == nil {
		return nil, func() error { return nil }
	}

	session := basiscodec.New()

	return context.WithValue(ctx, basisSessionKey{}, session), session.Close
}

func useBasis(ctx context.Context, operation func(*basiscodec.Session) error) error {
	if ctx == nil {
		return errBlockCompression
	}

	if session, ok := ctx.Value(basisSessionKey{}).(*basiscodec.Session); ok {
		return operation(session)
	}

	session := basiscodec.New()
	defer func() { _ = session.Close() }()

	return operation(session)
}

func encodeBasisImage(ctx context.Context, image texturedata.Data, quality int, etc1s bool) (output []byte, err error) {
	request := basiscodec.Image{
		Width:  uint32(image.Width),
		Height: uint32(image.Height),
		Levels: image.Levels,
		SRGB:   image.ColorSpace == texturedata.SRGB,
	}
	err = useBasis(ctx, func(session *basiscodec.Session) error {
		var failure error

		output, failure = session.Encode(ctx, request, quality, etc1s)

		return failure
	})

	return output, err
}

func transcodeBasisBlocks(ctx context.Context, data []byte, info blockUASTCInfo, format uint32) (levels [][]byte, err error) {
	target := uint64(0)

	switch format {
	case blockVKBC1, blockVKBC1SRGB:
		target = 2
	case blockVKBC7, blockVKBC7SRGB:
		target = 6
	case blockVKASTC4x4:
		target = 10
	case blockVKETC2RGBA, blockVKETC2RGBASRGB:
		target = 1
	case blockVKETC2RGB, blockVKETC2RGBSRGB:
		target = 0
	case blockVKBC6H:
		target = 22
	default:
		return nil, errBlockCompression
	}

	err = useBasis(ctx, func(session *basiscodec.Session) error {
		result, failure := session.Transcode(ctx, data, target)
		if failure != nil {
			return failure
		}

		if int(result.Width) != info.width || int(result.Height) != info.height || len(result.Levels) != info.levels ||
			result.SRGB != (info.space == texturedata.SRGB) {
			return errBlockCompression
		}

		levels = result.Levels

		return nil
	})

	return levels, err
}

// PrepareETC1S encodes admitted opaque sRGB RGBA8 with its authored mip chain.
// This runs only during asset preparation and never installs an external tool.
func PrepareETC1S(ctx context.Context, source []byte, quality int) ([]byte, error) {
	bounded, cancel, err := blockContext(ctx, 0)
	if err != nil {
		return nil, err
	}
	defer cancel()

	image, err := opaqueLDRBlockInput(source)
	if err != nil || image.ColorSpace != texturedata.SRGB || quality < 1 || quality > 255 {
		return nil, errBlockCompression
	}

	output, err := encodeBasisImage(bounded, image, quality, true)
	if err != nil {
		return nil, err
	}

	info, err := basistranscode.Inspect(output)
	if err != nil {
		return nil, err
	}

	if info.Width != image.Width || info.Height != image.Height || info.Levels != len(image.Levels) {
		return nil, errBlockCompression
	}

	return output, nil
}

func compressRGBEGo(ctx context.Context, source []byte, options BlockCompressionOptions) (output []byte, err error) {
	image, err := blockInput(source)
	if err != nil || len(image.Levels) != 1 || options.HDREffort < 0 || options.HDREffort > 4 {
		return nil, errBlockCompression
	}

	bounded, cancel, err := blockContext(ctx, options.Timeout)
	if err != nil {
		return nil, err
	}
	defer cancel()

	pixels, err := rgbeFloatPixels(image)
	if err != nil {
		return nil, err
	}

	request := basiscodec.Image{Width: uint32(image.Width), Height: uint32(image.Height), Levels: [][]byte{pixels}, HDR: true}

	err = useBasis(bounded, func(session *basiscodec.Session) error {
		prepared, failure := session.Encode(bounded, request, options.HDREffort, false)
		if failure != nil {
			return failure
		}

		result, failure := session.Transcode(bounded, prepared, 22)
		if failure != nil {
			return failure
		}

		if int(result.Width) != image.Width || int(result.Height) != image.Height || len(result.Levels) != 1 || result.SRGB {
			return errBlockCompression
		}

		output = writeNativeBlockKTX(blockVKBC6H, image.Width, image.Height, result.Levels)

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("offline WASI HDR: %w", err)
	}

	if len(output) < 80 || binary.LittleEndian.Uint32(output[12:]) != blockVKBC6H {
		return nil, errBlockCompression
	}

	return output, nil
}
