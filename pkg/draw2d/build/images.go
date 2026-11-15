//go:build !js

// Package build prepares 2D images through the host asset pipeline. It must not
// be imported by native or browser game runtime packages.
package build

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/karty-game/karty-webgpu/internal/ktx2data"
	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
	"github.com/karty-game/karty-webgpu/pkg/texture"
)

const (
	filesSlot       = "files"
	pluginName      = "draw2d-images"
	pageChannels    = 4
	pageOpaqueAlpha = 255
	maxPageQuality  = 4
)

var errImages = errors.New("invalid 2D image catalog")

// ImagesOptions selects offline page formats without changing dimensions,
// atlas layout or alpha semantics. Zero values preserve raw RGBA8 output.
type ImagesOptions struct {
	// OpaqueFormat selects RGBA8, BC1 or BC7 for pages whose alpha is all 255.
	OpaqueFormat texture.Format
	// AlphaFormat selects RGBA8 or BC7 for pages with any nonopaque alpha.
	AlphaFormat texture.Format
	Compression assetbuild.BlockCompressionOptions
}

type imagesPlugin struct {
	options ImagesOptions
	err     error
}

// Images publishes straight-alpha GPU-ready pages and a typed assets.Catalog for 2D
// images. Put it after assetbuild.Images and optional Resize/Atlas stages, without
// KTX2 or GoCatalog. Each generated Load call admits each page once, then shares
// it across named ImageAsset regions. Source codecs and atlas packing stay offline.
// With no options it publishes raw RGBA8. Optional BC1/BC7 encoding runs offline.
func Images(options ...ImagesOptions) assetbuild.Plugin {
	plugin := imagesPlugin{}
	if len(options) > 1 {
		plugin.err = fmt.Errorf("draw2d/build.Images accepts one options record: %w", errImages)
	} else if len(options) == 1 {
		plugin.options = options[0]
	}

	return plugin
}

func (imagesPlugin) Name() string { return pluginName }

func (plugin imagesPlugin) Plan(plan *assetbuild.Plan) error {
	if plugin.err != nil {
		return plugin.err
	}

	if err := validateImagesOptions(plugin.options); err != nil {
		return err
	}

	inputs := make([]assetbuild.Ref, 0, len(plan.Resources))
	for _, resource := range plan.Resources {
		schema, err := plan.SlotSchema(resource.Ref)
		if err != nil {
			return err
		}

		if schema == assetbuild.PreparedSetSchema {
			return fmt.Errorf("draw2d/build.Images must consume images before KTX2: %w", errImages)
		}

		if schema == assetbuild.ImageSetSchema {
			inputs = append(inputs, resource.Ref)
		}
	}

	refs, err := plan.Add(assetbuild.Action{
		ID: pluginName, Processor: pluginName, Revision: "prepared-page-catalog@2-wasi1", Options: plugin.options,
		Inputs: inputs, Outputs: []assetbuild.Slot{{Name: filesSlot, Schema: "karty.files@1"}},
		Run: func(ctx *assetbuild.Context) (assetbuild.Result, error) {
			set, readErr := readImages(ctx, inputs)
			if readErr != nil {
				return nil, readErr
			}

			files, catalogErr := imageCatalog(ctx, set, plugin.options)
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

func imageCatalog(ctx context.Context, set assetbuild.ImageSet, options ImagesOptions) (assetbuild.Bundle, error) {
	formats := imageFormats(set, options)

	code, err := catalogCode(set, formats...)
	if err != nil {
		return nil, err
	}

	files := assetbuild.Bundle{"assets.go": code}

	for index, image := range set.Images {
		pixels, err := preparePage(ctx, image, formats[index], options.Compression)
		if err != nil {
			return nil, fmt.Errorf("prepare 2D image %q: %w", image.Name, err)
		}

		files[pageFilename(image.Name, formats[index])] = pixels
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

func pageFilename(name string, format texture.Format) string {
	extension := ".rgba"

	switch format {
	case texture.BC1:
		extension = ".bc1"
	case texture.BC7:
		extension = ".bc7"
	case texture.RGBA8, texture.ETC2RGB, texture.BC6H, texture.ETC2RGBA, texture.ASTC4x4:
		// Option admission prevents unsupported compressed formats reaching publication.
	}

	return "pages/" + name + extension
}

func validateImagesOptions(options ImagesOptions) error {
	if options.OpaqueFormat != texture.RGBA8 && options.OpaqueFormat != texture.BC1 && options.OpaqueFormat != texture.BC7 ||
		options.AlphaFormat != texture.RGBA8 && options.AlphaFormat != texture.BC7 || options.Compression.Timeout < 0 {
		return fmt.Errorf("unsupported 2D page format or compression timeout: %w", errImages)
	}

	if quality := options.Compression.LDRQuality; quality != nil && (*quality < 0 || *quality > maxPageQuality) {
		return fmt.Errorf("invalid 2D compression quality: %w", errImages)
	}

	return nil
}

func imageFormats(set assetbuild.ImageSet, options ImagesOptions) []texture.Format {
	formats := make([]texture.Format, len(set.Images))
	for index, image := range set.Images {
		formats[index] = options.OpaqueFormat

		for _, level := range image.Levels {
			for alpha := pageChannels - 1; alpha < len(level); alpha += pageChannels {
				if level[alpha] != pageOpaqueAlpha {
					formats[index] = options.AlphaFormat

					break
				}
			}
		}
	}

	return formats
}

func preparePage(
	ctx context.Context,
	image assetbuild.Image,
	format texture.Format,
	options assetbuild.BlockCompressionOptions,
) ([]byte, error) {
	if format == texture.RGBA8 {
		return image.Levels[0], nil
	}

	source, err := assetbuild.EncodeTexture(image)
	if err != nil {
		return nil, err
	}

	var encoded []byte
	if format == texture.BC1 {
		encoded, err = assetbuild.CompressBC1(ctx, source, options)
	} else {
		encoded, err = assetbuild.CompressBC7(ctx, source, options)
	}

	if err != nil {
		return nil, err
	}

	data, err := ktx2data.Decode(encoded)
	if err != nil || data.Format != format || data.Width != image.Width || data.Height != image.Height || len(data.Levels) != 1 {
		return nil, errImages
	}

	return data.Levels[0], nil
}
