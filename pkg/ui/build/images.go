//go:build !js

// Package build prepares UI assets through the host asset pipeline. It must not
// be imported by native or browser game runtime packages.
package build

import (
	"errors"

	"github.com/karty-game/ashura/internal/imagecatalog"
	"github.com/karty-game/ashura/pkg/assetbuild"
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
	inputs, err := imagecatalog.Inputs(plan, "ui/build.Images", errImages)
	if err != nil {
		return err
	}

	refs, err := plan.Add(assetbuild.Action{
		Parallel: true,
		ID:       pluginName, Processor: pluginName, Revision: "raw-rgba-catalog@1",
		Inputs: inputs, Outputs: []assetbuild.Slot{{Name: filesSlot, Schema: "karty.files@1"}},
		Run: func(ctx *assetbuild.Context) (assetbuild.Result, error) {
			set, readErr := imagecatalog.Read(ctx, inputs)
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
	plan.DescribeImageFields(inputs, "assets.Load()")

	return nil
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

	if err := imagecatalog.AddAtlasMetadata(files, set.Regions); err != nil {
		return nil, err
	}

	return files, nil
}

func pageFilename(name string) string { return "pages/" + name + ".rgba" }
