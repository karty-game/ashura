//go:build !js

package impostorbuild

import (
	"context"
	"fmt"

	"github.com/karty-game/ashura/internal/impostordata"
	"github.com/karty-game/ashura/pkg/assetbuild"
)

// ETC2 retains the original straight-alpha color and RGB normal/depth-alpha
// layout, which the existing shader already consumes for non-BC1 captures.
func compressMobileAtlases(ctx context.Context, document impostordata.Document) (impostordata.Document, error) {
	color, err := assetbuild.CompressETC2RGBA(ctx, document.Color, assetbuild.BlockCompressionOptions{})
	if err != nil {
		return impostordata.Document{}, fmt.Errorf("ETC2 impostor color: %w", err)
	}

	normal, err := assetbuild.CompressETC2RGBA(ctx, document.NormalDepth, assetbuild.BlockCompressionOptions{})
	if err != nil {
		return impostordata.Document{}, fmt.Errorf("ETC2 impostor normal/depth: %w", err)
	}

	document.Color, document.NormalDepth, document.Storage = color, normal, impostordata.StorageETC2RGBA

	return document, nil
}

func compressASTCAtlases(ctx context.Context, document impostordata.Document) (impostordata.Document, error) {
	color, err := assetbuild.CompressASTC6x6(ctx, document.Color, assetbuild.BlockCompressionOptions{})
	if err != nil {
		return impostordata.Document{}, fmt.Errorf("ASTC impostor color: %w", err)
	}

	normal, err := assetbuild.CompressASTC6x6(ctx, document.NormalDepth, assetbuild.BlockCompressionOptions{})
	if err != nil {
		return impostordata.Document{}, fmt.Errorf("ASTC impostor normal/depth: %w", err)
	}

	document.Color, document.NormalDepth, document.Storage = color, normal, impostordata.StorageASTC6x6

	return document, nil
}
