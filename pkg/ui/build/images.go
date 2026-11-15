//go:build !js

// Package build prepares UI assets through the host asset pipeline. It must not
// be imported by native or browser game runtime packages.
package build

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
)

const (
	filesSlot  = "files"
	pluginName = "ui-images"
)

var errImages = errors.New("invalid UI image catalog")

type imagesPlugin struct{}

// Images publishes straight-alpha RGBA8 pages and a typed assets.Catalog for UI
// images. Put it after assetbuild.Images and optional Resize/Atlas stages, without
// KTX2 or GoCatalog. Each generated Load call admits each page once, then shares
// it across named ImageAsset regions. Source codecs and atlas packing stay offline.
func Images() assetbuild.Plugin { return imagesPlugin{} }

func (imagesPlugin) Name() string { return pluginName }

func (imagesPlugin) Plan(plan *assetbuild.Plan) error {
	inputs := make([]assetbuild.Ref, 0, len(plan.Resources))
	for _, resource := range plan.Resources {
		schema, err := plan.SlotSchema(resource.Ref)
		if err != nil {
			return err
		}

		if schema == assetbuild.PreparedSetSchema {
			return fmt.Errorf("ui/build.Images must consume images before KTX2: %w", errImages)
		}

		if schema == assetbuild.ImageSetSchema {
			inputs = append(inputs, resource.Ref)
		}
	}

	refs, err := plan.Add(assetbuild.Action{
		Parallel: true,
		ID:       pluginName, Processor: pluginName, Revision: "raw-rgba-catalog@1",
		Inputs: inputs, Outputs: []assetbuild.Slot{{Name: filesSlot, Schema: "karty.files@1"}},
		Run: func(ctx *assetbuild.Context) (assetbuild.Result, error) {
			set, readErr := readImages(ctx, inputs)
			if readErr != nil {
				return nil, readErr
			}

			files, catalogErr := imageCatalog(set)
			if catalogErr != nil {
				return nil, catalogErr
			}

			data, encodeErr := assetbuild.EncodeBundle(files)
			if encodeErr != nil {
				return nil, encodeErr
			}

			return assetbuild.Result{filesSlot: data}, nil
		},
	})
	if err != nil {
		return err
	}

	plan.Publish(refs[0])

	return nil
}

func readImages(ctx *assetbuild.Context, inputs []assetbuild.Ref) (assetbuild.ImageSet, error) {
	var result assetbuild.ImageSet

	for _, input := range inputs {
		data, err := ctx.Input(input)
		if err != nil {
			return assetbuild.ImageSet{}, err
		}

		set, err := assetbuild.DecodeImageSet(data)
		if err != nil {
			return assetbuild.ImageSet{}, err
		}

		result.Images = append(result.Images, set.Images...)
		result.Regions = append(result.Regions, set.Regions...)
	}

	slices.SortFunc(result.Images, func(a, b assetbuild.Image) int { return strings.Compare(a.Name, b.Name) })
	slices.SortFunc(result.Regions, func(a, b assetbuild.AtlasRegion) int { return strings.Compare(a.Name, b.Name) })

	return result, nil
}

func imageCatalog(set assetbuild.ImageSet) (assetbuild.Bundle, error) {
	code, err := catalogCode(set)
	if err != nil {
		return nil, err
	}

	files := assetbuild.Bundle{"assets.go": code}
	for _, image := range set.Images {
		files[pageFilename(image.Name)] = image.Levels[0]
	}

	if len(set.Regions) > 0 {
		metadata, err := json.MarshalIndent(set.Regions, "", "  ")
		if err != nil {
			return nil, err
		}

		files["atlas.json"] = append(metadata, '\n')
	}

	return files, nil
}

func pageFilename(name string) string { return "pages/" + name + ".rgba" }
