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

	"github.com/karty-game/ashura/pkg/texture"
)

// KTX2Options selects offline native texture storage without changing source
// dimensions or authored mips. Zero Format retains RGBA8. UsageFormats overrides
// Format for a declared image role, such as UsageAlbedo or UsageNormal.
// BC1 has one-bit alpha (threshold 128); BC7 and ETC2RGBA preserve smooth alpha.
// UsageSSBump may select ASTC4x4 with independent linear RGB coefficients.
type KTX2Options struct {
	Format       texture.Format
	UsageFormats map[string]texture.Format
	Compression  BlockCompressionOptions
}

type ktxPlugin struct {
	options []KTX2Options
	mobile  *KTX2Options
}

// KTX2 encodes image resources using their authored mip chains. With no options
// it writes RGBA8; one options value may select offline BC1, BC7 or ETC2 blocks.
// Put Mipmaps before it to generate additional levels.
func KTX2(options ...KTX2Options) Plugin { return ktxPlugin{options: options} }

// KTX2Variants publishes both profiles from the same prepared images. A
// different mobile format adds a file under mobile/ and an accessor with the
// Mobile suffix. Both profiles use the primary asset when their formats match.
func KTX2Variants(primary, mobile KTX2Options) Plugin {
	return ktxPlugin{options: []KTX2Options{primary}, mobile: &mobile}
}

func (ktxPlugin) Name() string { return "ktx2" }

func (plugin ktxPlugin) Plan(plan *Plan) error {
	options, err := normalizedKTX2Options(plugin.options)
	if err != nil {
		return err
	}

	var actionOptions any = options

	var mobile *KTX2Options
	if plugin.mobile != nil {
		normalized, err := normalizedKTX2Options([]KTX2Options{*plugin.mobile})
		if err != nil {
			return err
		}

		mobile = &normalized
		actionOptions = struct{ Primary, Mobile KTX2Options }{options, normalized}
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
			Options: actionOptions, Inputs: []Ref{resource.Ref},
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

				return prepareImageSet(ctx, set, options, mobile)
			},
		})
		if err != nil {
			return err
		}

		plan.Resources[index].Ref = refs[0]
		plan.carryInspectedImages(resource.Ref, refs[1])
		plan.Publish(refs[2])
	}

	return nil
}

func prepareImageSet(ctx context.Context, set ImageSet, options KTX2Options, mobile *KTX2Options) (Result, error) {
	prepared := PreparedSet{Regions: set.Regions}
	metadata := PreparedSet{Regions: set.Regions}
	files := make(Bundle)

	for _, value := range set.Images {
		data, err := EncodeTexture(value)
		if err != nil {
			return nil, err
		}

		primaryFormat := imageFormat(options, value.Usage)

		entry, err := prepareImageVariant(ctx, value, data, primaryFormat, options, false)
		if err != nil {
			return nil, err
		}

		if err := addPreparedImage(entry, &metadata, files); err != nil {
			return nil, err
		}

		prepared.Images = append(prepared.Images, entry)

		if mobile == nil {
			continue
		}

		mobileFormat := imageFormat(*mobile, value.Usage)
		if mobileFormat == primaryFormat {
			continue
		}

		entry, err = prepareImageVariant(ctx, value, data, mobileFormat, *mobile, true)
		if err != nil {
			return nil, err
		}

		if err := addPreparedImage(entry, &metadata, files); err != nil {
			return nil, err
		}
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

func imageFormat(options KTX2Options, usage string) texture.Format {
	if format, exists := options.UsageFormats[usage]; exists {
		return format
	}

	return options.Format
}

func prepareImageVariant(
	ctx context.Context, image Image, source []byte, format texture.Format, options KTX2Options, mobile bool,
) (PreparedImage, error) {
	name, output := image.Name, textureOutput(image.Source)
	if mobile {
		name += "Mobile"
		output = "mobile/" + output
	}

	data, err := compressPreparedImage(ctx, source, image.Usage, format, options.Compression)
	if err != nil {
		return PreparedImage{}, fmt.Errorf("compress texture %q: %w", output, err)
	}

	return PreparedImage{Name: name, Source: image.Source, Usage: image.Usage, Output: output, Data: data}, nil
}

func addPreparedImage(entry PreparedImage, metadata *PreparedSet, files Bundle) error {
	if strings.ContainsAny(entry.Output, "*?[\\") {
		return fmt.Errorf("texture %q embed path: %w", entry.Output, errImageRecord)
	}

	if _, exists := files[entry.Output]; exists {
		return fmt.Errorf("duplicate texture output %q: %w", entry.Output, errImageRecord)
	}

	files[entry.Output] = entry.Data
	entry.Data = nil
	metadata.Images = append(metadata.Images, entry)

	return nil
}

func compressPreparedImage(
	ctx context.Context,
	data []byte,
	usage string,
	format texture.Format,
	options BlockCompressionOptions,
) ([]byte, error) {
	switch format {
	case texture.RGBA8:
		return data, nil
	case texture.BC1:
		return CompressBC1(ctx, data, options)
	case texture.BC7:
		return CompressBC7(ctx, data, options)
	case texture.ETC2RGB:
		return CompressETC2RGB(ctx, data, options)
	case texture.ETC2RGBA:
		return CompressETC2RGBA(ctx, data, options)
	case texture.ASTC4x4:
		if usage == UsageSSBump {
			return CompressSSBumpASTC(ctx, data, options)
		}
	case texture.BC6H, texture.ASTC6x6:
	}

	return nil, errBlockCompression
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
		return format == texture.RGBA8 || format == texture.BC1 || format == texture.BC7 ||
			format == texture.ETC2RGB || format == texture.ETC2RGBA
	}
	if !validFormat(options.Format) || options.Compression.Timeout < 0 {
		return KTX2Options{}, errBlockCompression
	}

	if _, err := blockUASTCQuality(options.Compression.LDRQuality, 0); err != nil {
		return KTX2Options{}, err
	}

	for usage, format := range options.UsageFormats {
		if !validUsage(usage) || usage == UsageHeight || (!validFormat(format) && (usage != UsageSSBump || format != texture.ASTC4x4)) {
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
