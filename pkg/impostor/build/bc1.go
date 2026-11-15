//go:build !js

package impostorbuild

import (
	"context"
	"errors"
	"fmt"

	"github.com/karty-game/karty-webgpu/internal/impostorbake"
	"github.com/karty-game/karty-webgpu/internal/impostordata"
	"github.com/karty-game/karty-webgpu/internal/ktx2data"
	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
)

var errBC1AtlasDimensions = errors.New("BC1 impostor atlas dimensions must be multiples of four")

// Resolve public capture defaults before examining model data or adding actions.
// Raw internal capture layouts remain useful independently of BC1 publication.
func captureSettings(options Options) (impostorbake.Options, error) {
	settings, err := impostorbake.Defaults(options.settings())
	if err != nil || settings.Mode == impostordata.ModeLayered {
		return settings, err
	}

	columns, rows := settings.Azimuths, settings.Elevations
	if settings.Mode == impostordata.ModeOctahedral {
		columns, rows = settings.FrameGrid, settings.FrameGrid
	}

	const paddingSides, blockSide = 2, 4

	cell := settings.Resolution + paddingSides*settings.Padding

	width, height := columns*cell, rows*cell
	if width%blockSide != 0 || height%blockSide != 0 {
		return impostorbake.Options{}, fmt.Errorf("%w: %dx%d", errBC1AtlasDimensions, width, height)
	}

	return settings, nil
}

// View captures keep their normal-aware mip generation in the rasterizer.
// Only the final prepared mips are packed and compressed for publication.
func compressViewAtlases(ctx context.Context, document impostordata.Document) (impostordata.Document, error) {
	color, err := assetbuild.CompressBC1(ctx, document.Color, assetbuild.BlockCompressionOptions{})
	if err != nil {
		return impostordata.Document{}, fmt.Errorf("BC1 impostor color: %w", err)
	}

	packed, err := packedNormalDepth(ctx, document.NormalDepth, document.Source)
	if err != nil {
		return impostordata.Document{}, err
	}

	normalDepth, err := assetbuild.CompressBC1(ctx, packed, assetbuild.BlockCompressionOptions{})
	if err != nil {
		return impostordata.Document{}, fmt.Errorf("BC1 impostor normal/depth: %w", err)
	}

	document.Color, document.NormalDepth, document.Storage = color, normalDepth, impostordata.StorageBC1

	return document, nil
}

func packedNormalDepth(ctx context.Context, source []byte, name string) ([]byte, error) {
	data, err := ktx2data.Decode(source)
	if err != nil {
		return nil, err
	}

	levels := make([][]byte, len(data.Levels))
	for index, mip := range data.Levels {
		levels[index], err = impostorbake.PackBC1NormalDepth(ctx, mip)
		if err != nil {
			return nil, err
		}
	}

	return assetbuild.EncodeTexture(assetbuild.Image{
		Name: "ImpostorPackedNormalDepth", Source: name, Usage: assetbuild.UsageAO,
		Width: data.Width, Height: data.Height, Levels: levels, MaxMipLevels: len(levels), Atlas: true,
	})
}
