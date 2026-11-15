package level

import (
	"github.com/karty-game/karty-webgpu/pkg/scene/resource"
	"github.com/karty-game/karty-webgpu/pkg/texture"
)

// TextureVariants identifies offline-prepared device profiles of one level.
// Select after App.Run has opened the device, before reading either manifest.
// Only the selected profile's resources are requested by the normal chunk loader.
type TextureVariants struct{ BC, Mobile resource.Asset }

// Select reuses App.CompressedAlbedoFormat's device snapshot; it performs one
// enum selection, no I/O, allocations, texture transformations or codec work.
func (variants TextureVariants) Select(format texture.Format) (resource.Asset, error) {
	switch format {
	case texture.BC1:
		return variants.BC, nil
	case texture.ETC2RGB:
		return variants.Mobile, nil
	case texture.RGBA8, texture.BC6H, texture.BC7, texture.ETC2RGBA, texture.ASTC4x4:
		return resource.Asset{}, ErrNativeHDR
	default:
		return resource.Asset{}, ErrNativeHDR
	}
}
