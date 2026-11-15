//go:build !js

package impostorbake

import (
	"context"
	"fmt"

	"github.com/karty-game/karty-webgpu/internal/impostordata"
	"github.com/karty-game/karty-webgpu/internal/modeldata"
	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
)

const (
	defaultIntervals   = 8
	intervalBoundaries = 2
	quadtreeDivisor    = 3
	quadtreeSide       = 2
	quadtreeChildren   = 4
)

func layeredDefaults(options Options) (Options, error) {
	if options.Resolution == 0 {
		options.Resolution = defaultResolution
	}

	if options.Padding == 0 {
		options.Padding = defaultPadding
	}

	if options.MipLevels == 0 {
		options.MipLevels = 1
	}

	if options.MaxIntervals == 0 {
		options.MaxIntervals = defaultIntervals
	}

	if options.FrameGrid != 0 || options.Azimuths != 0 || options.Elevations != 0 || options.ElevationMin != 0 ||
		options.ElevationMax != 0 ||
		options.MipLevels != 1 ||
		options.Resolution < 32 ||
		options.Resolution > 256 ||
		options.Padding < 1 ||
		options.Padding > 128 ||
		options.MaxIntervals < 1 ||
		options.MaxIntervals > impostordata.MaximumIntervals {
		return Options{}, errBounds
	}

	_, err := impostordata.LayeredLayout(uint32(options.Resolution), uint32(options.Padding), uint32(options.MaxIntervals))
	if err != nil {
		return Options{}, errBounds
	}

	return options, nil
}

//nolint:gosec // Defaults bounds resolution, padding and interval counts before this internal bake path.
func bakeLayered(
	ctx context.Context,
	source modeldata.Document,
	geometry geometry,
	options Options,
	report *Report,
) (impostordata.Document, error) {
	layout, err := impostordata.LayeredLayout(uint32(options.Resolution), uint32(options.Padding), uint32(options.MaxIntervals))
	if err != nil {
		return impostordata.Document{}, err
	}

	document := descriptor(source, geometry, options)
	document.Azimuths, document.Elevations, document.Columns = uint32(
		options.MaxIntervals,
	), impostordata.LayeredAxes, impostordata.LayeredColumns
	document.AtlasWidth, document.AtlasHeight = layout.Width, layout.Height
	document.Views = impostordata.LayeredViews()
	color, numeric := make(
		[]byte,
		int(layout.Width)*int(layout.Height)*channels,
	), make(
		[]byte,
		int(layout.Width)*int(layout.Height)*channels,
	)

	for axis, view := range document.Views {
		hits, err := layeredRaster(ctx, geometry, view, options.Resolution)
		if err != nil {
			return impostordata.Document{}, err
		}

		if err := packLayeredAxis(geometry, hits, color, numeric, layout, options, axis, report); err != nil {
			return impostordata.Document{}, fmt.Errorf("axis%d: %w", axis, err)
		}
	}

	document.Color, err = assetbuild.EncodeTexture(
		assetbuild.Image{
			Name:         "ImpostorColor",
			Source:       source.Source,
			Usage:        assetbuild.UsageAlbedo,
			Width:        int(layout.Width),
			Height:       int(layout.Height),
			Levels:       [][]byte{color},
			MaxMipLevels: 1,
			Atlas:        true,
		},
	)
	if err != nil {
		return impostordata.Document{}, err
	}

	document.NormalDepth, err = assetbuild.EncodeTexture(
		assetbuild.Image{
			Name:         "ImpostorIntervals",
			Source:       source.Source,
			Usage:        assetbuild.UsageAO,
			Width:        int(layout.Width),
			Height:       int(layout.Height),
			Levels:       [][]byte{numeric},
			MaxMipLevels: 1,
			Atlas:        true,
		},
	)

	return document, err
}
