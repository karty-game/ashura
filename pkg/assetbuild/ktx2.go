//go:build !js

package assetbuild

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"maps"
	"path"
	"strings"

	"github.com/karty-game/karty-webgpu/pkg/texture"
)

// KTX2Options selects offline native texture storage without changing source
// dimensions or authored mips. Zero Format retains RGBA8. UsageFormats overrides
// Format for a declared image role, such as UsageAlbedo or UsageNormal.
// BC1 has one-bit alpha (threshold 128); use BC7 for smooth transparency.
type KTX2Options struct {
	Format       texture.Format
	UsageFormats map[string]texture.Format
	Compression  BlockCompressionOptions
}

type ktxPlugin struct{ options []KTX2Options }

// KTX2 encodes image resources using their authored mip chains. With no options
// it writes RGBA8; one options value may explicitly select offline BC1 or BC7.
// Put Mipmaps before it to generate additional levels.
func KTX2(options ...KTX2Options) Plugin { return ktxPlugin{options: options} }

func (ktxPlugin) Name() string { return "ktx2" }

func (plugin ktxPlugin) Plan(plan *Plan) error {
	options, err := normalizedKTX2Options(plugin.options)
	if err != nil {
		return err
	}

	for index, resource := range plan.Resources {
		applicable, schemaErr := imageResourceApplies(plan, resource.Ref, ImageSetSchema)
		if schemaErr != nil {
			return schemaErr
		}

		if !applicable {
			continue
		}

		refs, err := plan.Add(Action{
			Parallel: true,
			ID:       imageActionID(plan, "ktx2/"+resource.ID), Processor: "ktx2", Revision: "native-blocks@2",
			Options: options, Inputs: []Ref{resource.Ref},
			Outputs: []Slot{
				{Name: imageDataSlot, Schema: PreparedSetSchema},
				{Name: imageCatalogSlot, Schema: "karty.image-catalog@1"},
				{Name: imageFilesSlot, Schema: "karty.files@1"},
			},
			Run: func(ctx *Context) (Result, error) {
				set, inputErr := imageInput(ctx, resource.Ref)
				if inputErr != nil {
					return nil, inputErr
				}

				return prepareImageSet(ctx, set, options)
			},
		})
		if err != nil {
			return err
		}

		plan.Resources[index].Ref = refs[0]
		plan.Publish(refs[2])
	}

	return nil
}

func prepareImageSet(ctx context.Context, set ImageSet, options KTX2Options) (Result, error) {
	prepared := PreparedSet{Regions: set.Regions}
	metadata := PreparedSet{Regions: set.Regions}
	files := make(Bundle)

	for _, value := range set.Images {
		data, err := EncodeTexture(value)
		if err != nil {
			return nil, err
		}

		format := options.Format
		if selected, exists := options.UsageFormats[value.Usage]; exists {
			format = selected
		}

		switch format {
		case texture.RGBA8:
		case texture.BC1:
			data, err = CompressBC1(ctx, data, options.Compression)
		case texture.BC7:
			data, err = CompressBC7(ctx, data, options.Compression)
		case texture.ETC2RGB, texture.BC6H, texture.ETC2RGBA, texture.ASTC4x4:
			return nil, errBlockCompression
		default:
			return nil, errBlockCompression
		}

		if err != nil {
			return nil, fmt.Errorf("compress texture %q: %w", value.Source, err)
		}

		output := textureOutput(value.Source)
		if strings.ContainsAny(output, "*?[\\") {
			return nil, fmt.Errorf("texture %q embed path: %w", output, errImageRecord)
		}

		if _, exists := files[output]; exists {
			return nil, fmt.Errorf("duplicate texture output %q: %w", output, errImageRecord)
		}

		entry := PreparedImage{Name: value.Name, Source: value.Source, Usage: value.Usage, Output: output, Data: data}
		prepared.Images = append(prepared.Images, entry)
		entry.Data = nil
		metadata.Images = append(metadata.Images, entry)
		files[output] = data
	}

	data, err := json.Marshal(prepared)
	if err != nil {
		return nil, err
	}

	meta, err := json.Marshal(metadata)
	if err != nil {
		return nil, err
	}

	payloads, err := json.Marshal(files)
	if err != nil {
		return nil, err
	}

	return Result{imageDataSlot: data, imageCatalogSlot: meta, imageFilesSlot: payloads}, nil
}

func normalizedKTX2Options(values []KTX2Options) (KTX2Options, error) {
	if len(values) > 1 {
		return KTX2Options{}, errBlockCompression
	}

	var options KTX2Options
	if len(values) == 1 {
		options = values[0]
	}

	options.UsageFormats = maps.Clone(options.UsageFormats)

	validFormat := func(format texture.Format) bool {
		return format == texture.RGBA8 || format == texture.BC1 || format == texture.BC7
	}
	if !validFormat(options.Format) || options.Compression.Timeout < 0 {
		return KTX2Options{}, errBlockCompression
	}

	if _, err := blockUASTCQuality(options.Compression.LDRQuality, 0); err != nil {
		return KTX2Options{}, err
	}

	for usage, format := range options.UsageFormats {
		if !validUsage(usage) || usage == UsageHeight || !validFormat(format) {
			return KTX2Options{}, errBlockCompression
		}
	}

	return options, nil
}

func textureOutput(source string) string {
	extension := path.Ext(source)
	if sourceImageExtension(extension) {
		source = strings.TrimSuffix(source, extension)
	}

	return source + ".ktx2"
}

// EncodeTexture validates and writes the bounded RGBA8 KTX2 profile understood
// by the optional runtime loader. No compression or transcoding is performed.
func EncodeTexture(value Image) ([]byte, error) {
	if value.Usage == UsageHeight {
		return nil, fmt.Errorf("height source %q must pass through SSBump before KTX2: %w", value.Source, errImageRecord)
	}

	if err := validateImageSet(ImageSet{Images: []Image{value}}); err != nil {
		return nil, err
	}

	width, height := value.Width, value.Height

	levels := make([]*image.NRGBA, len(value.Levels))
	for index, pixels := range value.Levels {
		levels[index] = &image.NRGBA{Pix: pixels, Stride: width * imageChannels, Rect: image.Rect(0, 0, width, height)}
		width, height = max(1, width>>1), max(1, height>>1)
	}

	return encodeKTX(levels, value.Usage == UsageAlbedo || value.Usage == UsageReflection), nil
}
