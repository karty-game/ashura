//go:build !js

package assetbuild

import (
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strings"
)

const (
	defaultAtlasSize  = 2048
	maximumAtlasSize  = 4096
	maximumAtlasMips  = 13
	atlasChannels     = 4
	maximumAtlasBytes = 96 << 20
)

var errAtlas = errors.New("invalid atlas configuration or image")

// AtlasOptions selects one explicitly clamp-compatible atlas group. Include
// matches logical resource IDs. MipLevels defaults to one; optional extra levels
// require sufficient extruded padding and aligned packing, enforced by Atlas.
type AtlasOptions struct {
	Name      string
	Include   []string
	MaxSize   int
	Padding   int
	MipLevels int
	Clamp     bool
}

type atlasPlugin struct{ options AtlasOptions }

// Atlas packs selected images offline into role-separated pages and UV regions.
// Clamp must be explicitly true: repeating material maps cannot be packed safely.
// Run Mipmaps after Atlas to generate the requested, safely bounded mip levels.
func Atlas(options AtlasOptions) Plugin { return atlasPlugin{options: options} }

func (plugin atlasPlugin) Name() string { return "atlas" }

func (plugin atlasPlugin) Plan(plan *Plan) error {
	options, err := normalizedAtlasOptions(plugin.options)
	if err != nil {
		return err
	}

	selected, retained, err := atlasImageResources(plan, options.Include)
	if err != nil {
		return err
	}

	plan.inspectAtlas(options, selected)

	if len(selected) == 0 {
		return nil
	}

	inputs := make([]Ref, 0, len(selected))
	for _, resource := range selected {
		if err := requireResourceSchema(plan, resource.Ref, ImageSetSchema); err != nil {
			return err
		}

		inputs = append(inputs, resource.Ref)
	}

	refs, err := plan.Add(Action{
		Parallel:  true,
		ID:        "atlas/" + options.Name,
		Processor: "atlas",
		Revision:  "1",
		Options:   options,
		Inputs:    inputs,
		Outputs:   []Slot{{Name: imageDataSlot, Schema: ImageSetSchema}},
		Run: func(ctx *Context) (Result, error) {
			return runAtlas(ctx, inputs, options)
		},
	})
	if err != nil {
		return err
	}

	plan.inspectAtlasImages(refs[0], inputs, options.Name)

	retained = append(retained, Resource{ID: "atlas/" + options.Name, Ref: refs[0]})
	plan.Resources = retained
	slices.SortFunc(plan.Resources, compareAtlasResources)

	return nil
}

func atlasImageResources(plan *Plan, patterns []string) (selected, retained []Resource, err error) {
	candidates, retained, err := selectAtlasResources(plan.Resources, patterns)
	if err != nil {
		return nil, nil, err
	}

	for _, resource := range candidates {
		applicable, schemaErr := imageResourceApplies(plan, resource.Ref, ImageSetSchema)
		if schemaErr != nil {
			return nil, nil, schemaErr
		}

		if applicable {
			selected = append(selected, resource)
		} else {
			retained = append(retained, resource)
		}
	}

	return selected, retained, nil
}

func normalizedAtlasOptions(options AtlasOptions) (AtlasOptions, error) {
	if options.Name == "" {
		options.Name = "Atlas"
	}

	if options.MaxSize == 0 {
		options.MaxSize = defaultAtlasSize
	}

	if options.MipLevels == 0 {
		options.MipLevels = 1
	}

	if !options.Clamp || len(options.Include) == 0 || !fs.ValidPath(options.Name) || strings.Contains(options.Name, "/") ||
		AssetName(options.Name) == "" || options.MaxSize < 1 || options.MaxSize > maximumAtlasSize ||
		options.MaxSize&(options.MaxSize-1) != 0 || options.MipLevels < 1 || options.MipLevels > maximumAtlasMips {
		return AtlasOptions{}, fmt.Errorf(
			"atlas requires a name, Include, Clamp=true, power-of-two MaxSize <=4096 and valid MipLevels: %w",
			errAtlas,
		)
	}

	alignment := 1 << (options.MipLevels - 1)
	if options.Padding == 0 {
		options.Padding = alignment
	}

	if options.Padding < alignment || options.Padding > options.MaxSize/2 {
		return AtlasOptions{}, fmt.Errorf(
			"atlas padding must be >=%d for %d mip levels and fit MaxSize: %w",
			alignment,
			options.MipLevels,
			errAtlas,
		)
	}

	options.Include = slices.Clone(options.Include)
	for _, pattern := range options.Include {
		if pattern == "" {
			return AtlasOptions{}, fmt.Errorf("atlas include patterns must not be empty: %w", errAtlas)
		}

		if _, err := MatchAsset(pattern, ""); err != nil {
			return AtlasOptions{}, fmt.Errorf("atlas include %q: %w", pattern, err)
		}
	}

	return options, nil
}

func selectAtlasResources(resources []Resource, patterns []string) (selected, retained []Resource, err error) {
	for _, resource := range resources {
		matches := false

		for _, pattern := range patterns {
			match, matchErr := MatchAsset(pattern, resource.ID)
			if matchErr != nil {
				return nil, nil, fmt.Errorf("atlas include %q: %w", pattern, matchErr)
			}

			matches = matches || match
		}

		if matches {
			selected = append(selected, resource)
		} else {
			retained = append(retained, resource)
		}
	}

	slices.SortFunc(selected, compareAtlasResources)

	return selected, retained, nil
}

func compareAtlasResources(left, right Resource) int { return strings.Compare(left.ID, right.ID) }

func runAtlas(ctx *Context, inputs []Ref, options AtlasOptions) (Result, error) {
	var images []Image

	for _, input := range inputs {
		data, err := ctx.Input(input)
		if err != nil {
			return nil, err
		}

		set, err := DecodeImageSet(data)
		if err != nil {
			return nil, err
		}

		if len(set.Regions) != 0 {
			return nil, fmt.Errorf("atlas input already contains packed regions: %w", errAtlas)
		}

		images = append(images, set.Images...)
	}

	set, err := packAtlas(images, options)
	if err != nil {
		return nil, err
	}

	data, err := EncodeImageSet(set)
	if err != nil {
		return nil, err
	}

	return Result{imageDataSlot: data}, nil
}
