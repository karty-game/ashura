//go:build !js

package build

import (
	"context"
	"crypto/sha256"
	"time"

	"github.com/karty-game/ashura/internal/ktx2data"
	"github.com/karty-game/ashura/internal/msdfcodec"
	"github.com/karty-game/ashura/pkg/assetbuild"
	"github.com/karty-game/ashura/pkg/texture"
)

const fontToolTimeout = 2 * time.Minute

func prepareDefaultFontFormat(ctx *assetbuild.Context, format texture.Format) (assetbuild.Result, error) {
	bounded, cancel := context.WithTimeout(ctx, fontToolTimeout)
	defer cancel()

	pair, err := msdfcodec.PrepareGoRegular(bounded)
	if err != nil {
		return nil, err
	}

	if err = validateDefaultFont(pair.Metrics, pair.Atlas); err != nil {
		return nil, err
	}

	filename := "atlas.rgba"

	if format == texture.BC7 {
		source, failure := assetbuild.EncodeTexture(assetbuild.Image{
			// EncodeTexture only uses this numeric role to select linear transfer;
			// no relief filtering or processing stage runs on the distance page.
			Name: "FontAtlas", Source: "atlas.png", Usage: assetbuild.UsageSSBump,
			Width: fontAtlasSide, Height: fontAtlasSide, Levels: [][]byte{pair.Atlas},
		})
		if failure != nil {
			return nil, failure
		}
		// Effort 2 bounds preparation of the full-resolution page within the
		// existing font deadline; distance channels are kept independent.
		quality := 2

		encoded, failure := assetbuild.CompressBC7(bounded, source, assetbuild.BlockCompressionOptions{LDRQuality: &quality})
		if failure != nil {
			return nil, failure
		}

		prepared, failure := ktx2data.Decode(encoded)
		if failure != nil || prepared.Format != texture.BC7 || prepared.ColorSpace != texture.Linear ||
			prepared.Width != fontAtlasSide || prepared.Height != fontAtlasSide || len(prepared.Levels) != 1 {
			return nil, errDefaultFont
		}

		pair.Atlas = prepared.Levels[0]
		hash := sha256.Sum256(pair.Atlas)
		copy(pair.Metrics[60:92], hash[:])

		if failure := validateCompressedDefaultFont(pair.Metrics, pair.Atlas); failure != nil {
			return nil, failure
		}

		filename = "atlas.bc7"
	}

	data, err := assetbuild.EncodeBundle(assetbuild.Bundle{filename: pair.Atlas, "glyphs.bin": pair.Metrics})

	return assetbuild.Result{filesSlot: data}, err
}
