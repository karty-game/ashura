//go:build !js

package assetbuild

import (
	"errors"
	"fmt"
	"math"
)

var errResize = errors.New("invalid image resize options")

// ResizeOptions selects exact dimensions or an aspect-preserving maximum.
// A single Width or Height preserves aspect ratio. MaxDimension never upscales.
// Include matches source-relative logical identities; an empty list selects all.
type ResizeOptions struct {
	Width, Height int
	MaxDimension  int
	Include       []string
}

type resizePlugin struct{ options ResizeOptions }

// Resize scales source images offline with role-aware filtering. Place it before
// Atlas and Mipmaps; atlas records cannot be resized without rebuilding mappings.
func Resize(options ResizeOptions) Plugin { return resizePlugin{options: options} }

func (resizePlugin) Name() string { return "resize" }

func (plugin resizePlugin) Plan(plan *Plan) error {
	if err := validateResizeOptions(plugin.options); err != nil {
		return err
	}

	return transformImages(plan, "resize", "area-linear@1", plugin.options, plugin.options.Include, plugin.resizeSet)
}

func validateResizeOptions(options ResizeOptions) error {
	if options.Width < 0 || options.Height < 0 || options.MaxDimension < 0 ||
		options.Width > maxImageDimension || options.Height > maxImageDimension || options.MaxDimension > maxImageDimension ||
		options.MaxDimension > 0 && (options.Width > 0 || options.Height > 0) ||
		options.Width == 0 && options.Height == 0 && options.MaxDimension == 0 {
		return errResize
	}

	return nil
}

// ResizeImage prepares one base-only image with the same role-aware filtering
// as Resize. It is intended for offline importers with embedded source images;
// Include is a pipeline selector and must be empty for this direct operation.
func ResizeImage(value Image, options ResizeOptions) (Image, error) {
	if err := validateResizeOptions(options); err != nil {
		return Image{}, err
	}

	if len(options.Include) != 0 || len(value.Levels) != 1 {
		return Image{}, errResize
	}

	if err := validateImageSet(ImageSet{Images: []Image{value}}); err != nil {
		return Image{}, err
	}

	result, err := (resizePlugin{options: options}).resizeSet(ImageSet{Images: []Image{value}})
	if err != nil {
		return Image{}, err
	}

	return result.Images[0], nil
}

func (plugin resizePlugin) resizeSet(set ImageSet) (ImageSet, error) {
	if len(set.Regions) > 0 {
		return ImageSet{}, fmt.Errorf("resize must precede atlas packing: %w", errResize)
	}

	for index, value := range set.Images {
		if value.Atlas {
			return ImageSet{}, fmt.Errorf("cannot resize atlas %q: %w", value.Name, errResize)
		}

		width, height := resizeDimensions(value.Width, value.Height, plugin.options)
		if width < 1 || height < 1 || width > maxImageDimension || height > maxImageDimension {
			return ImageSet{}, errResize
		}

		if width != value.Width || height != value.Height {
			value.Levels = [][]byte{resampleImage(value, width, height)}
			value.Width, value.Height = width, height
		}

		set.Images[index] = value
	}

	return set, nil
}

func resizeDimensions(width, height int, options ResizeOptions) (int, int) {
	if options.MaxDimension > 0 {
		if max(width, height) <= options.MaxDimension {
			return width, height
		}

		if width >= height {
			return options.MaxDimension, max(1, int(math.Round(float64(height)*float64(options.MaxDimension)/float64(width))))
		}

		return max(1, int(math.Round(float64(width)*float64(options.MaxDimension)/float64(height)))), options.MaxDimension
	}

	if options.Width == 0 {
		return max(1, int(math.Round(float64(width)*float64(options.Height)/float64(height)))), options.Height
	}

	if options.Height == 0 {
		return options.Width, max(1, int(math.Round(float64(height)*float64(options.Width)/float64(width))))
	}

	return options.Width, options.Height
}

// resampleImage uses box-area filtering for reduction and bilinear filtering for
// enlargement, independently on each axis. Pixel colors use their declared role.
func resampleImage(value Image, width, height int) []byte {
	pixels := make([]byte, width*height*imageChannels)
	for row := range height {
		rows := resizeWeights(row, height, value.Height)
		for column := range width {
			columns := resizeWeights(column, width, value.Width)

			var sum [imageChannels]float64

			for _, sampleRow := range rows {
				for _, sampleColumn := range columns {
					index := (sampleRow.index*value.Width + sampleColumn.index) * imageChannels
					for channel, sample := range value.Levels[0][index : index+imageChannels] {
						decoded := float64(sample) / byteMaximum

						if channel < colorChannels {
							switch value.Usage {
							case UsageAlbedo:
								decoded = toLinear(decoded)
							case UsageNormal:
								decoded = decoded*normalRange - 1
							}
						}

						sum[channel] += decoded * sampleColumn.weight * sampleRow.weight
					}
				}
			}

			encodeResampled(pixels[(row*width+column)*imageChannels:], sum, value.Usage)
		}
	}

	return pixels
}

type resizeWeight struct {
	index  int
	weight float64
}

func resizeWeights(index, size, source int) []resizeWeight {
	scale := float64(source) / float64(size)
	if scale <= 1 {
		position := (float64(index)+halfRange)*scale - halfRange
		left := int(math.Floor(position))
		fraction := position - float64(left)

		return []resizeWeight{{max(0, min(source-1, left)), 1 - fraction}, {max(0, min(source-1, left+1)), fraction}}
	}

	start, end := float64(index)*scale, float64(index+1)*scale

	weights := make([]resizeWeight, 0, int(math.Ceil(scale))+1)
	for sample := int(math.Floor(start)); sample < int(math.Ceil(end)); sample++ {
		weight := (min(end, float64(sample+1)) - max(start, float64(sample))) / scale
		weights = append(weights, resizeWeight{min(source-1, sample), weight})
	}

	return weights
}

func encodeResampled(pixels []byte, sum [imageChannels]float64, usage string) {
	if usage == UsageNormal {
		length := math.Sqrt(sum[0]*sum[0] + sum[1]*sum[1] + sum[2]*sum[2])
		if length > 0 {
			for channel := range colorChannels {
				sum[channel] /= length
			}
		} else {
			sum[2] = 1
		}
	}

	for channel, value := range sum {
		if channel < colorChannels {
			switch usage {
			case UsageAlbedo:
				value = toSRGB(value)
			case UsageNormal:
				value = value*halfRange + halfRange
			}
		}

		pixels[channel] = byte(math.Round(max(0, min(byteMaximum, value*byteMaximum))))
	}
}
