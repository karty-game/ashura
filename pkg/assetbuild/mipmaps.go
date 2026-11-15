//go:build !js

package assetbuild

import "image"

type mipmapPlugin struct{}

// Mipmaps creates a role-aware mip chain for each current image resource.
// Explicit atlas mip limits are preserved; a base-only atlas remains base-only.
func Mipmaps() Plugin { return mipmapPlugin{} }

func (mipmapPlugin) Name() string { return "mipmaps" }

func (mipmapPlugin) Plan(plan *Plan) error {
	return transformImages(plan, "mipmaps", "role-filter@1", nil, nil, func(set ImageSet) (ImageSet, error) {
		for index, value := range set.Images {
			prepared, err := GenerateMipmaps(value)
			if err != nil {
				return ImageSet{}, err
			}

			set.Images[index] = prepared
		}

		return set, nil
	})
}

// GenerateMipmaps prepares all permitted levels, filtering albedo in linear
// light, normal vectors with renormalization and AO/SSBump as linear numbers.
func GenerateMipmaps(value Image) (Image, error) {
	if err := validateImageSet(ImageSet{Images: []Image{value}}); err != nil {
		return Image{}, err
	}

	base := image.NewNRGBA(image.Rect(0, 0, value.Width, value.Height))
	copy(base.Pix, value.Levels[0])
	levels := mipmaps(base, value.Usage, value.MaxMipLevels)

	value.Levels = make([][]byte, len(levels))
	for index, level := range levels {
		value.Levels[index] = level.Pix
	}

	return value, nil
}
