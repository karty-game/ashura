//go:build !js

package assetbuild

import (
	"context"
	"errors"
	"fmt"
	"image"
	"math"
	"path"
	"slices"
	"strings"

	"github.com/karty-game/ashura/internal/ssbump"
)

var errSSBump = errors.New("invalid SSBump source image or options")

// SSBumpOptions selects normal/height resources and bounded offline visibility work.
// Zero numeric fields select 300 rays, 32 source texels and 0.05 height bias.
// Include matches Resource.ID; an empty list selects every current resource.
// Other image roles in a selected ImageSet are preserved. KeepNormals retains both
// the original normal and derived SSBump; otherwise the derived map replaces it.
type SSBumpOptions struct {
	Include     []string
	Rays        int
	Distance    float64
	Bias        float64
	KeepNormals bool
	// HeightScale is the relief in output texels for the full [0,1] range of
	// authored *-height maps. Zero selects eight texels. Height sources are
	// consumed, never uploaded; KeepNormals only retains normal sources.
	HeightScale float64
}

type ssbumpPlugin struct{ options SSBumpOptions }

// SSBump bakes squared RNM coefficients and per-lobe visibility from periodic
// normal-map relief or authored grayscale heights offline. Place it before Atlas and Mipmaps. It preserves
// resource grouping IDs while deriving *-ssbump source and catalog names.
func SSBump(options SSBumpOptions) Plugin { return ssbumpPlugin{options: options} }

func (ssbumpPlugin) Name() string { return "ssbump" }

func (plugin ssbumpPlugin) Plan(plan *Plan) error {
	options, err := normalizedSSBumpOptions(plugin.options)
	if err != nil {
		return err
	}

	for index, resource := range plan.Resources {
		selected, selectErr := selectedImage(options.Include, resource.ID)
		if selectErr != nil {
			return selectErr
		}

		if !selected {
			continue
		}

		applicable, schemaErr := imageResourceApplies(plan, resource.Ref, ImageSetSchema)
		if schemaErr != nil {
			return schemaErr
		}

		if !applicable {
			continue
		}

		refs, addErr := plan.Add(Action{
			Parallel: true,
			ID:       imageActionID(plan, "ssbump/"+resource.ID), Processor: "ssbump", Revision: "periodic-rnm-height@3",
			Options: options, Inputs: []Ref{resource.Ref}, Outputs: []Slot{{Name: imageDataSlot, Schema: ImageSetSchema}},
			Run: func(ctx *Context) (Result, error) {
				set, inputErr := imageInput(ctx, resource.Ref)
				if inputErr != nil {
					return nil, inputErr
				}

				prepared, bakeErr := bakeSSBumpSet(ctx, set, options)
				if bakeErr != nil {
					return nil, bakeErr
				}

				return imageResult(prepared)
			},
		})
		if addErr != nil {
			return addErr
		}

		plan.Resources[index].Ref = refs[0]
	}

	return nil
}

func normalizedSSBumpOptions(options SSBumpOptions) (SSBumpOptions, error) {
	settings, err := ssbump.NormalizeOptions(ssbump.Options{Rays: options.Rays, Distance: options.Distance, Bias: options.Bias})
	if err != nil {
		return SSBumpOptions{}, err
	}

	options.Rays, options.Distance, options.Bias = settings.Rays, settings.Distance, settings.Bias
	if options.HeightScale == 0 {
		options.HeightScale = 8
	}

	if math.IsNaN(options.HeightScale) || math.IsInf(options.HeightScale, 0) || options.HeightScale <= 0 || options.HeightScale > 512 {
		return SSBumpOptions{}, errSSBump
	}

	options.Include = slices.Clone(options.Include)
	for _, pattern := range options.Include {
		if _, err := MatchAsset(pattern, ""); err != nil {
			return SSBumpOptions{}, fmt.Errorf("SSBump selection %q: %w", pattern, err)
		}
	}

	return options, nil
}

func bakeSSBumpSet(ctx context.Context, set ImageSet, options SSBumpOptions) (ImageSet, error) {
	if len(set.Regions) != 0 {
		return ImageSet{}, fmt.Errorf("SSBump must precede atlas packing: %w", errSSBump)
	}

	var result ImageSet

	for _, value := range set.Images {
		if value.Usage != UsageNormal && value.Usage != UsageHeight {
			result.Images = append(result.Images, value)

			continue
		}

		if value.Atlas || len(value.Levels) != 1 {
			return ImageSet{}, fmt.Errorf("SSBump normal %s must have one unpacked level: %w", value.Name, errSSBump)
		}

		prepared, err := bakeSSBumpImage(ctx, value, options)
		if err != nil {
			return ImageSet{}, fmt.Errorf("SSBump normal %s: %w", value.Name, err)
		}

		if options.KeepNormals && value.Usage == UsageNormal {
			result.Images = append(result.Images, value)
		}

		result.Images = append(result.Images, prepared)
	}

	return result, nil
}

func bakeSSBumpImage(ctx context.Context, value Image, options SSBumpOptions) (Image, error) {
	// Normal directions live in RGB. Preserve straight channels when the numeric
	// image carries unrelated alpha; image.Color.RGBA otherwise premultiplies them.
	pixels := slices.Clone(value.Levels[0])
	for index := imageChannels - 1; index < len(pixels); index += imageChannels {
		pixels[index] = byteMaximum
	}

	normal := &image.NRGBA{
		Pix: pixels, Stride: value.Width * imageChannels,
		Rect: image.Rect(0, 0, value.Width, value.Height),
	}

	settings := ssbump.Options{Rays: options.Rays, Distance: options.Distance, Bias: options.Bias}

	var (
		prepared *image.NRGBA
		err      error
	)
	if value.Usage == UsageHeight {
		prepared, err = ssbump.GenerateHeight(ctx, normal, options.HeightScale, settings)
	} else {
		prepared, err = ssbump.Generate(ctx, normal, settings)
	}

	if err != nil {
		return Image{}, err
	}

	source := ssbumpSource(value.Source)
	stem, _ := ssbumpSourceParts(source)

	return Image{
		Name: AssetName(stem), Source: source, Usage: UsageSSBump,
		Width: value.Width, Height: value.Height, Levels: [][]byte{prepared.Pix},
	}, nil
}

func ssbumpSource(source string) string {
	stem, extension := ssbumpSourceParts(source)

	base := strings.ToLower(path.Base(stem))
	switch {
	case base == UsageNormal:
		stem = strings.TrimSuffix(stem, path.Base(stem)) + UsageSSBump
	case strings.HasSuffix(base, "-"+UsageNormal), strings.HasSuffix(base, "_"+UsageNormal):
		stem = stem[:len(stem)-len(UsageNormal)] + UsageSSBump
	case base == UsageHeight:
		stem = strings.TrimSuffix(stem, path.Base(stem)) + UsageSSBump
	case strings.HasSuffix(base, "-"+UsageHeight), strings.HasSuffix(base, "_"+UsageHeight):
		stem = stem[:len(stem)-len(UsageHeight)] + UsageSSBump
	default:
		stem += "-" + UsageSSBump
	}

	return stem + extension
}

func ssbumpSourceParts(source string) (string, string) {
	extension := path.Ext(source)
	if sourceImageExtension(extension) {
		return strings.TrimSuffix(source, extension), extension
	}

	return source, ""
}
