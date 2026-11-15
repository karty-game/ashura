package level

import (
	"github.com/karty-game/karty-webgpu/internal/leveldata"
	"github.com/karty-game/karty-webgpu/pkg/texture"
)

// This selects existing native upload/sampling formats. The complete manifest
// has already admitted the exact storage/transfer pair before any payload I/O.
func lightmapImageProfile(page Page) (texture.Format, texture.ColorSpace) {
	space := texture.Linear
	if page.Encoding == leveldata.EncodingSRGB {
		space = texture.SRGB
	}

	switch page.Storage {
	case nativeHDRStorage:
		return texture.BC6H, space
	case mobileStorage:
		return texture.ETC2RGBA, space
	case leveldata.StorageBC1LDR:
		return texture.BC1, space
	case leveldata.StorageBC7LDR:
		return texture.BC7, space
	case leveldata.StorageETC2LDR:
		return texture.ETC2RGB, space
	default:
		return texture.RGBA8, space
	}
}
