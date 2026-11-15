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

	"github.com/karty-game/karty-webgpu/internal/impostorbake"
	"github.com/karty-game/karty-webgpu/internal/impostordata"
	"github.com/karty-game/karty-webgpu/internal/modeldata"
	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
	modelbuild "github.com/karty-game/karty-webgpu/pkg/model/build"
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

		if err := addCard(plan, resource, settings); err != nil {
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

	return bakePreparedReport(ctx, preparedModel, settings)
}

func bakePrepared(ctx context.Context, preparedModel []byte, settings impostorbake.Options) ([]byte, error) {
	data, _, err := bakePreparedReport(ctx, preparedModel, settings)

	return data, err
}

func bakePreparedReport(ctx context.Context, preparedModel []byte, settings impostorbake.Options) ([]byte, Report, error) {
	source, err := modeldata.Decode(preparedModel)
	if err != nil {
		return nil, Report{}, err
	}

	result, report, err := impostorbake.BakeWithReport(ctx, source, settings)
	if err != nil {
		return nil, report, err
	}

	if result.Mode != impostordata.ModeLayered {
		result, err = compressViewAtlases(ctx, result)
		if err != nil {
			return nil, report, err
		}
	}

	data, err := impostordata.Encode(result)

	return data, report, err
}

func addCard(plan *assetbuild.Plan, resource assetbuild.Resource, settings impostorbake.Options) error {
	name := assetbuild.AssetName(strings.TrimSuffix(resource.ID, path.Ext(resource.ID)))

	refs, err := plan.Add(assetbuild.Action{
		ID:        "impostor/" + resource.ID,
		Processor: "impostors",
		Revision:  "bc1-view-atlases@9",
		Options:   settings,
		Inputs:    []assetbuild.Ref{resource.Ref},
		Outputs:   []assetbuild.Slot{{Name: preparedSlot, Schema: PreparedSchema}, {Name: filesSlot, Schema: "karty.files@1"}},
		Run: func(ctx *assetbuild.Context) (assetbuild.Result, error) {
			data, err := ctx.Input(resource.Ref)
			if err != nil {
				return nil, err
			}

			prepared, err := bakePrepared(ctx, data, settings)
			if err != nil {
				return nil, fmt.Errorf("impostor %s: %w", resource.ID, err)
			}

			file := "impostor_" + name + ".kimp"

			code, err := catalog(name, resource.ID, file)
			if err != nil {
				return nil, err
			}

			bundle, err := json.Marshal(assetbuild.Bundle{"impostors/" + file: prepared, "impostors/impostor_" + name + ".go": code})
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
	plan.Resources = append(plan.Resources, assetbuild.Resource{ID: resource.ID + "/impostor", Ref: refs[0]})

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
