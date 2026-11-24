//go:build !js

// Package impostorbuild adds offline layered and multi-view impostor baking to asset recipes.
// Runtime games use package impostor and never import this build-only package.
package impostorbuild

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/karty-game/ashura/internal/impostorbake"
	"github.com/karty-game/ashura/internal/impostordata"
	"github.com/karty-game/ashura/internal/modeldata"
	"github.com/karty-game/ashura/pkg/assetbuild"
	modelbuild "github.com/karty-game/ashura/pkg/model/build"
	"github.com/karty-game/ashura/pkg/texture"
)

const (
	// PreparedSchema is the immutable binary impostor resource schema.
	PreparedSchema = "karty.impostor@4"
	preparedSlot   = "impostor"
	filesSlot      = "files"
)

const (
	// Angular selects the legacy azimuth/elevation capture grid.
	Angular = "angular"
	// Octahedral selects full-sphere captures with fixed-cost three-view reprojection.
	Octahedral = "octahedral"
	// Layered selects occupied intervals and an offline numeric hierarchy.
	Layered = "layered"
)

// Options selects prepared models and their offline capture settings. Empty Mode
// defaults to an 8x8 octahedral grid of 80px captures. Existing angular-specific
// fields select Angular for recipe compatibility. Angular defaults to 128px, 8x3 views and
// -30..30 degrees. Padding4 permits three independently filtered mip levels.
// View atlas base dimensions must be multiples of four for BC1 publication.
// Layered must be selected explicitly; it requires closed solids, power-of-two
// Resolution32..256 and one numeric mip, with at most MaxIntervals occupied intervals.
type Options struct {
	// TextureProfile publishes a separate public payload and resource descriptor
	// under "bc" or "mobile". Empty retains the embedded catalog.
	TextureProfile string `json:"textureProfile,omitempty"`
	// AtlasFormat selects BC1 (the zero-value default), ETC2RGBA or ASTC6x6
	// for view captures. Mobile formats keep RGB normals and depth alpha.
	// Layered retains RGBA8.
	AtlasFormat texture.Format `json:"atlasFormat,omitempty"`
	// MaxIntervals bounds layered occupancy per axis pixel; zero selects eight.
	MaxIntervals int      `json:"maxIntervals,omitempty"`
	FrameGrid    int      `json:"frameGrid,omitempty"`
	Mode         string   `json:"mode,omitempty"`
	Include      []string `json:"include,omitempty"`
	Resolution   int      `json:"resolution"`
	Azimuths     int      `json:"azimuths"`
	Elevations   int      `json:"elevations"`
	Padding      int      `json:"padding"`
	MipLevels    int      `json:"mipLevels"`
	ElevationMin float32  `json:"elevationMin"`
	ElevationMax float32  `json:"elevationMax"`
}

type cardsPlugin struct{ options Options }

// Cards bakes cached, flattened modelbuild.GLB resources into prebuilt proxy
// geometry with BC1 color/coverage and oct-normal/depth atlases. Put GLB before it.
// Layered numeric occupancy retains its exact RGBA8 representation.
// All capture modes use one offline-prepared quad.
// Original model bundles remain available for ordinary model rendering.
func Cards(options Options) assetbuild.Plugin {
	options.Include = slices.Clone(options.Include)

	return cardsPlugin{options: options}
}

func (cardsPlugin) Name() string { return "impostors" }

func (plugin cardsPlugin) Plan(plan *assetbuild.Plan) error {
	settings, err := captureSettings(plugin.options)
	if err != nil {
		return err
	}

	for _, pattern := range plugin.options.Include {
		if _, err := assetbuild.MatchAsset(pattern, ""); err != nil {
			return err
		}
	}

	resources := slices.Clone(plan.Resources)
	for _, resource := range resources {
		schema, err := plan.SlotSchema(resource.Ref)
		if err != nil {
			return err
		}

		if schema != modelbuild.PreparedSchema {
			continue
		}

		matched, err := selected(plugin.options.Include, resource.ID)
		if err != nil {
			return err
		}

		if !matched {
			continue
		}

		if err := addCard(plan, resource, settings, plugin.options.AtlasFormat, plugin.options.TextureProfile); err != nil {
			return err
		}
	}

	return nil
}

func (options Options) settings() impostorbake.Options {
	mode := options.Mode
	if mode == "" {
		mode = Octahedral
		if options.Azimuths != 0 || options.Elevations != 0 || options.ElevationMin != 0 || options.ElevationMax != 0 {
			mode = Angular
		}
	}

	selectedMode := impostordata.ModeLayered + 1

	switch mode {
	case Angular:
		selectedMode = impostordata.ModeAngular
	case Octahedral:
		selectedMode = impostordata.ModeOctahedral
	case Layered:
		selectedMode = impostordata.ModeLayered
	}

	return impostorbake.Options{
		MaxIntervals: options.MaxIntervals,
		FrameGrid:    options.FrameGrid,
		Mode:         selectedMode,
		Resolution:   options.Resolution,
		Azimuths:     options.Azimuths,
		Elevations:   options.Elevations,
		Padding:      options.Padding,
		MipLevels:    options.MipLevels,
		ElevationMin: options.ElevationMin,
		ElevationMax: options.ElevationMax,
	}
}

// Bake serializes a prepared static model into a deterministic impostor container.
// It supports the same flattened model/prefab data used by Cards, without a CLI.
func Bake(ctx context.Context, preparedModel []byte, options Options) ([]byte, error) {
	data, _, err := BakeWithReport(ctx, preparedModel, options)

	return data, err
}

// Report exposes offline layered winding and interval diagnostics.
type Report = impostorbake.Report

// BakeWithReport serializes the resource and exposes tolerated numeric pockets.
func BakeWithReport(ctx context.Context, preparedModel []byte, options Options) ([]byte, Report, error) {
	settings, err := captureSettings(options)
	if err != nil {
		return nil, Report{}, err
	}

	return bakePreparedReport(ctx, preparedModel, settings, options.AtlasFormat)
}

func bakePrepared(ctx context.Context, preparedModel []byte, settings impostorbake.Options, format texture.Format) ([]byte, error) {
	data, _, err := bakePreparedReport(ctx, preparedModel, settings, format)

	return data, err
}

func bakePreparedReport(
	ctx context.Context,
	preparedModel []byte,
	settings impostorbake.Options,
	format texture.Format,
) ([]byte, Report, error) {
	source, err := modeldata.Decode(preparedModel)
	if err != nil {
		return nil, Report{}, err
	}

	result, report, err := impostorbake.BakeWithReport(ctx, source, settings)
	if err != nil {
		return nil, report, err
	}

	if result.Mode != impostordata.ModeLayered {
		switch format {
		case texture.ASTC6x6:
			result, err = compressASTCAtlases(ctx, result)
		case texture.ETC2RGBA:
			result, err = compressMobileAtlases(ctx, result)
		case texture.RGBA8, texture.BC1, texture.ETC2RGB, texture.BC6H, texture.BC7, texture.ASTC4x4:
			result, err = compressViewAtlases(ctx, result)
		}

		if err != nil {
			return nil, report, err
		}
	}

	data, err := impostordata.Encode(result)

	return data, report, err
}

func addCard(
	plan *assetbuild.Plan,
	resource assetbuild.Resource,
	settings impostorbake.Options,
	format texture.Format,
	profile string,
) error {
	name := assetbuild.AssetName(strings.TrimSuffix(resource.ID, path.Ext(resource.ID)))

	actionID := "impostor/" + resource.ID
	if profile != "" {
		actionID = "impostor/" + profile + "/" + resource.ID
	}

	refs, err := plan.Add(assetbuild.Action{
		ID:        actionID,
		Processor: "impostors",
		Revision:  "native-view-atlases@10",
		Options: struct {
			Capture impostorbake.Options
			Format  texture.Format
			Profile string
		}{settings, format, profile},
		Inputs:  []assetbuild.Ref{resource.Ref},
		Outputs: []assetbuild.Slot{{Name: preparedSlot, Schema: PreparedSchema}, {Name: filesSlot, Schema: "karty.files@1"}},
		Run: func(ctx *assetbuild.Context) (assetbuild.Result, error) {
			data, err := ctx.Input(resource.Ref)
			if err != nil {
				return nil, err
			}

			prepared, err := bakePrepared(ctx, data, settings, format)
			if err != nil {
				return nil, fmt.Errorf("impostor %s: %w", resource.ID, err)
			}

			files, err := cardFiles(name, resource.ID, prepared, profile)
			if err != nil {
				return nil, err
			}

			bundle, err := json.Marshal(files)
			if err != nil {
				return nil, err
			}

			return assetbuild.Result{preparedSlot: prepared, filesSlot: bundle}, nil
		},
	})
	if err != nil {
		return err
	}

	plan.Publish(refs[1])

	identity := resource.ID + "/impostor"
	if profile != "" {
		identity += "/" + profile
	}

	plan.Resources = append(plan.Resources, assetbuild.Resource{ID: identity, Ref: refs[0]})

	return nil
}

func selected(patterns []string, name string) (bool, error) {
	if len(patterns) == 0 {
		return true, nil
	}

	for _, pattern := range patterns {
		matched, err := assetbuild.MatchAsset(pattern, name)
		if err != nil {
			return false, err
		}

		if matched {
			return true, nil
		}
	}

	return false, nil
}
