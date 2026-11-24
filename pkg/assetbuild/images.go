//go:build !js

package assetbuild

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/draw"
	_ "image/jpeg"
	_ "image/png"
	"path"
	"slices"
	"strings"
)

const maxSourceBytes = 32 << 20

var errSourceImage = errors.New("invalid source image")

type imagePlugin struct{}

// Images imports PNG/JPEG files selected by Config.Input, inferring albedo,
// normal, height, AO and SSBump roles from source-relative filenames.
func Images() Plugin { return imagePlugin{} }

func (imagePlugin) Name() string { return "images" }

func (imagePlugin) Plan(plan *Plan) error {
	sources, err := discoverImages(plan)
	if err != nil {
		return err
	}

	for _, source := range sources {
		refs, addErr := plan.Add(Action{
			Parallel: true,
			ID:       "images/" + source.identity, Processor: "images", Revision: "rgba8@1",
			Options: source,
			Outputs: []Slot{{Name: imageDataSlot, Schema: ImageSetSchema}},
			Run: func(ctx *Context) (Result, error) {
				encoded, readErr := ctx.ReadFile(source.path)
				if readErr != nil {
					return nil, readErr
				}

				value, decodeErr := DecodeImage(encoded, source.name, source.relative, source.usage)
				if decodeErr != nil {
					return nil, decodeErr
				}

				return imageResult(ImageSet{Images: []Image{value}})
			},
		})
		if addErr != nil {
			return addErr
		}

		plan.Resources = append(plan.Resources, Resource{ID: source.identity, Ref: refs[0]})
		plan.inspectImage(refs[0], source)
	}

	return nil
}

type imageSource struct {
	path, relative, identity, name, usage string
}

// MarshalJSON includes effective input identities in an image action key.
func (source imageSource) MarshalJSON() ([]byte, error) {
	return marshalRecord([]string{source.path, source.relative, source.identity, source.name, source.usage})
}

func discoverImages(plan *Plan) ([]imageSource, error) {
	var result []imageSource

	paths, names := make(map[string]bool), make(map[string]string)

	for _, pattern := range plan.Input {
		files, err := plan.Glob(pattern)
		if err != nil {
			return nil, err
		}

		root := inputRoot(pattern)

		for _, file := range files {
			ext := strings.ToLower(path.Ext(file))
			if !sourceImageExtension(ext) {
				if plan.inspection != nil {
					plan.inspection.skipped[file] = true
				}

				continue
			}

			if paths[file] {
				continue
			}

			relative := strings.TrimPrefix(file, root)
			identity := strings.TrimSuffix(relative, path.Ext(relative))

			name := AssetName(identity)
			if name == "" || strings.ContainsAny(relative, "*?[\\") {
				return nil, fmt.Errorf("source %q has an unsupported asset name: %w", file, errSourceImage)
			}

			if first, exists := names[name]; exists {
				return nil, fmt.Errorf(
					"sources %q and %q both generate %q; rename one to give it a distinct Go name: %w",
					first,
					file,
					name,
					errSourceImage,
				)
			}

			paths[file], names[name] = true, file
			result = append(result, imageSource{file, relative, identity, name, imageUsage(identity)})
		}
	}

	slices.SortFunc(result, func(left, right imageSource) int { return strings.Compare(left.identity, right.identity) })

	return result, nil
}

func inputRoot(pattern string) string {
	index := strings.IndexAny(pattern, "*?[")
	if index >= 0 {
		pattern = pattern[:index]
	}

	index = strings.LastIndex(pattern, "/")
	if index < 0 {
		return ""
	}

	return pattern[:index+1]
}

func sourceImageExtension(extension string) bool {
	switch strings.ToLower(extension) {
	case ".png", ".jpg", authoringJPEGExtension:
		return true
	default:
		return false
	}
}

func imageUsage(identity string) string {
	name := strings.ToLower(path.Base(identity))
	if name == UsageReflection || strings.HasSuffix(name, "-"+UsageReflection) || strings.HasSuffix(name, "_"+UsageReflection) {
		return UsageReflection
	}

	for _, usage := range []string{UsageNormal, UsageHeight, UsageAO, UsageSSBump} {
		if name == usage || strings.HasSuffix(name, "-"+usage) || strings.HasSuffix(name, "_"+usage) {
			return usage
		}
	}

	return UsageAlbedo
}

// DecodeImage bounds and decodes a source PNG/JPEG to one immutable RGBA8 level.
func DecodeImage(encoded []byte, name, source, usage string) (Image, error) {
	if len(encoded) > maxSourceBytes || !validUsage(usage) {
		return Image{}, errSourceImage
	}

	config, _, err := image.DecodeConfig(bytes.NewReader(encoded))
	if err != nil {
		return Image{}, fmt.Errorf("source %s header: %w", source, err)
	}

	if config.Width < 1 || config.Height < 1 || config.Width > maxImageDimension || config.Height > maxImageDimension {
		return Image{}, fmt.Errorf("source %s exceeds 4096-pixel dimensions: %w", source, errSourceImage)
	}

	decoded, _, err := image.Decode(bytes.NewReader(encoded))
	if err != nil {
		return Image{}, fmt.Errorf("source %s pixels: %w", source, err)
	}

	rgba := image.NewNRGBA(image.Rect(0, 0, config.Width, config.Height))
	draw.Draw(rgba, rgba.Bounds(), decoded, decoded.Bounds().Min, draw.Src)

	return Image{
		Name: name, Source: source, Usage: usage,
		Width: config.Width, Height: config.Height, Levels: [][]byte{rgba.Pix},
	}, nil
}
