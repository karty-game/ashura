//go:build !js

// Package build prepares 2D images through the host asset pipeline. It must not
// be imported by native or browser game runtime packages.
package build

import (
	"context"
	"errors"
	"fmt"

	"github.com/karty-game/ashura/internal/imagecatalog"
	"github.com/karty-game/ashura/internal/ktx2data"
	"github.com/karty-game/ashura/pkg/assetbuild"
	"github.com/karty-game/ashura/pkg/texture"
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
	// OpaqueFormat selects RGBA8, BC1, BC7, ETC2RGB or ETC2RGBA for pages whose alpha is all 255.
	OpaqueFormat texture.Format
	// AlphaFormat selects RGBA8, BC7 or ETC2RGBA for pages with any nonopaque alpha.
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
// With no options it publishes raw RGBA8. Optional BC/ETC2 encoding runs offline.
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

	inputs, err := imagecatalog.Inputs(plan, "draw2d/build.Images", errImages)
	if err != nil {
		return err
	}

	if err := planImages(plan, inputs, plugin.options); err != nil {
		return err
	}

	plan.DescribeImageFields(inputs, "assets.Load()")

	return nil
}

func planImages(plan *assetbuild.Plan, inputs []assetbuild.Ref, options ImagesOptions) error {
	refs, err := plan.Add(assetbuild.Action{
		ID: pluginName, Processor: pluginName, Revision: "prepared-page-catalog@2-wasi1", Options: options,
		Inputs: inputs, Outputs: []assetbuild.Slot{{Name: filesSlot, Schema: "karty.files@1"}},
		Run: func(ctx *assetbuild.Context) (assetbuild.Result, error) {
			set, readErr := imagecatalog.Read(ctx, inputs)
			if readErr != nil {
				return nil, readErr
			}

			files, catalogErr := imageCatalog(ctx, set, imageFormats(set, options), options.Compression)
			if catalogErr != nil {
				return nil, catalogErr
			}

			return catalogResult(files)
		},
	})
	if err != nil {
		return err
	}

	plan.Publish(refs[0])

	return nil
}

func catalogResult(files assetbuild.Bundle) (assetbuild.Result, error) {
	data, err := assetbuild.EncodeBundle(files)
	if err != nil {
		return nil, err
	}

	return assetbuild.Result{filesSlot: data}, nil
}

func imageCatalog(
	ctx context.Context,
	set assetbuild.ImageSet,
	formats []texture.Format,
	compression assetbuild.BlockCompressionOptions,
) (assetbuild.Bundle, error) {
	code, err := catalogCode(set, formats...)
	if err != nil {
		return nil, err
	}

	files := assetbuild.Bundle{"assets.go": code}

	for index, image := range set.Images {
		pixels, err := preparePage(ctx, image, formats[index], compression)
		if err != nil {
			return nil, fmt.Errorf("prepare 2D image %q: %w", image.Name, err)
		}

		files[pageFilename(image.Name, formats[index])] = pixels
	}

	if err := imagecatalog.AddAtlasMetadata(files, set.Regions); err != nil {
		return nil, err
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
	case texture.ETC2RGB:
		extension = ".etc2rgb"
	case texture.ETC2RGBA:
		extension = ".etc2rgba"
	case texture.RGBA8, texture.BC6H, texture.ASTC4x4, texture.ASTC6x6:
		// Option admission prevents unsupported compressed formats reaching publication.
	}

	return "pages/" + name + extension
}

func validateImagesOptions(options ImagesOptions) error {
	if options.OpaqueFormat != texture.RGBA8 && options.OpaqueFormat != texture.BC1 && options.OpaqueFormat != texture.BC7 &&
		options.OpaqueFormat != texture.ETC2RGB && options.OpaqueFormat != texture.ETC2RGBA ||
		options.AlphaFormat != texture.RGBA8 && options.AlphaFormat != texture.BC7 &&
			options.AlphaFormat != texture.ETC2RGBA || options.Compression.Timeout < 0 {
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

	switch format {
	case texture.BC1:
		encoded, err = assetbuild.CompressBC1(ctx, source, options)
	case texture.BC7:
		encoded, err = assetbuild.CompressBC7(ctx, source, options)
	case texture.ETC2RGB:
		encoded, err = assetbuild.CompressETC2RGB(ctx, source, options)
	case texture.ETC2RGBA:
		encoded, err = assetbuild.CompressETC2RGBA(ctx, source, options)
	case texture.RGBA8, texture.BC6H, texture.ASTC4x4, texture.ASTC6x6:
		return nil, errImages
	default:
		return nil, errImages
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
