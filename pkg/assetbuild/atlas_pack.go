//go:build !js

package assetbuild

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

type atlasPlacement struct {
	image         Image
	x, y          int
	width, height int
}

type atlasPage struct {
	x, y, rowHeight       int
	usedWidth, usedHeight int
	placements            []atlasPlacement
}

func packAtlas(images []Image, options AtlasOptions) (ImageSet, error) {
	groups, err := groupAtlasImages(images)
	if err != nil {
		return ImageSet{}, err
	}

	usages := make([]string, 0, len(groups))
	for usage := range groups {
		usages = append(usages, usage)
	}

	slices.Sort(usages)

	var result ImageSet

	totalBytes := 0

	for _, usage := range usages {
		pages, packErr := layoutAtlas(groups[usage], options)
		if packErr != nil {
			return ImageSet{}, packErr
		}

		for index, page := range pages {
			width, height := atlasPowerOfTwo(page.usedWidth), atlasPowerOfTwo(page.usedHeight)

			totalBytes += width * height * atlasChannels
			if totalBytes > maximumAtlasBytes {
				return ImageSet{}, fmt.Errorf("atlas group exceeds 96 MiB page budget: %w", errAtlas)
			}

			image, regions := renderAtlasPage(page, width, height, usage, index, options)
			result.Images = append(result.Images, image)
			result.Regions = append(result.Regions, regions...)
		}
	}

	slices.SortFunc(result.Regions, func(left, right AtlasRegion) int { return strings.Compare(left.Name, right.Name) })

	return result, nil
}

func groupAtlasImages(images []Image) (map[string][]Image, error) {
	groups := make(map[string][]Image)
	names := make(map[string]bool)

	for _, image := range images {
		if image.Atlas || image.Name == "" || names[image.Name] || image.Width < 1 || image.Height < 1 ||
			image.Width > maximumAtlasSize || image.Height > maximumAtlasSize || len(image.Levels) != 1 ||
			len(image.Levels[0]) != image.Width*image.Height*atlasChannels || !validUsage(image.Usage) {
			return nil, fmt.Errorf("atlas input %q must be unique, bounded, unmipped and have a supported role: %w", image.Name, errAtlas)
		}

		names[image.Name] = true
		groups[image.Usage] = append(groups[image.Usage], image)
	}

	return groups, nil
}

func layoutAtlas(images []Image, options AtlasOptions) ([]atlasPage, error) {
	images = slices.Clone(images)
	slices.SortFunc(images, compareAtlasImages)

	alignment := 1 << (options.MipLevels - 1)

	var pages []atlasPage

	for _, image := range images {
		width := atlasAlign(image.Width+options.Padding*2, alignment)
		height := atlasAlign(image.Height+options.Padding*2, alignment)

		if width > options.MaxSize || height > options.MaxSize {
			return nil, fmt.Errorf(
				"atlas image %q including gutters does not fit %dx%d: %w",
				image.Name,
				options.MaxSize,
				options.MaxSize,
				errAtlas,
			)
		}

		placed := false

		for index := range pages {
			if pages[index].place(image, width, height, options.MaxSize) {
				placed = true

				break
			}
		}

		if !placed {
			page := atlasPage{}
			page.place(image, width, height, options.MaxSize)
			pages = append(pages, page)
		}
	}

	return pages, nil
}

func compareAtlasImages(left, right Image) int {
	if left.Height != right.Height {
		return right.Height - left.Height
	}

	if left.Width != right.Width {
		return right.Width - left.Width
	}

	return strings.Compare(left.Name, right.Name)
}

func (page *atlasPage) place(image Image, width, height, maximum int) bool {
	column, row, rowHeight := page.x, page.y, page.rowHeight
	if column+width > maximum {
		column, row, rowHeight = 0, row+rowHeight, 0
	}

	if row+height > maximum {
		return false
	}

	page.placements = append(page.placements, atlasPlacement{image: image, x: column, y: row, width: width, height: height})
	page.x, page.y, page.rowHeight = column+width, row, max(rowHeight, height)
	page.usedWidth, page.usedHeight = max(page.usedWidth, column+width), max(page.usedHeight, row+height)

	return true
}

func renderAtlasPage(page atlasPage, width, height int, usage string, index int, options AtlasOptions) (Image, []AtlasRegion) {
	identity := options.Name + "-" + usage + "-page-" + strconv.Itoa(index)
	image := Image{
		Name: AssetName(identity), Source: "atlas/" + identity, Usage: usage,
		Width: width, Height: height, Levels: [][]byte{make([]byte, width*height*atlasChannels)},
		MaxMipLevels: options.MipLevels, Atlas: true,
	}
	regions := make([]AtlasRegion, 0, len(page.placements))

	for _, placement := range page.placements {
		extrudeAtlasImage(image.Levels[0], width, placement, options.Padding)
		regions = append(regions, AtlasRegion{
			Name: placement.image.Name, Source: placement.image.Source, Page: image.Name,
			X: placement.x + options.Padding, Y: placement.y + options.Padding,
			Width: placement.image.Width, Height: placement.image.Height,
			PageWidth: width, PageHeight: height,
		})
	}

	return image, regions
}

func extrudeAtlasImage(destination []byte, stride int, placement atlasPlacement, padding int) {
	for y := range placement.height {
		sourceY := min(max(y-padding, 0), placement.image.Height-1)
		for x := range placement.width {
			sourceX := min(max(x-padding, 0), placement.image.Width-1)
			source := (sourceY*placement.image.Width + sourceX) * atlasChannels
			target := ((placement.y+y)*stride + placement.x + x) * atlasChannels
			copy(destination[target:target+atlasChannels], placement.image.Levels[0][source:source+atlasChannels])
		}
	}
}

func atlasAlign(value, alignment int) int { return (value + alignment - 1) / alignment * alignment }

func atlasPowerOfTwo(value int) int {
	result := 1
	for result < value {
		result <<= 1
	}

	return result
}
