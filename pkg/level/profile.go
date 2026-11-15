package level

import (
	"errors"

	"github.com/karty-game/karty-webgpu/internal/basistranscode"
	"github.com/karty-game/karty-webgpu/internal/leveldata"
	"github.com/karty-game/karty-webgpu/pkg/texture"
)

const (
	nativeHDRStorage = "bc-hdr"
	etc1sEncoding    = "etc1s"
	mobileStorage    = "etc2-astc"
	bc1Encoding      = "bc1"
	etc2Encoding     = "etc2"
)

// ErrNativeTextures rejects native assets without their selected device family.
var ErrNativeTextures = errors.New("level: native texture profile requires its selected device compression family")

// ErrNativeHDR is the compatibility name for native profile admission errors.
var ErrNativeHDR = ErrNativeTextures

// Reject unavailable encodings using the selected device snapshot before any
// payload I/O. There is no implicit raw-texture preload or runtime HDR codec.
func validateDeviceProfile(chunk Chunk, format texture.Format, decoderAvailable bool) error {
	for _, page := range chunk.Pages {
		if (page.Storage == nativeHDRStorage || page.Storage == leveldata.StorageBC1LDR || page.Storage == leveldata.StorageBC7LDR) &&
			format != texture.BC1 {
			return ErrNativeHDR
		}

		if (page.Storage == mobileStorage || page.Storage == leveldata.StorageETC2LDR) && format != texture.ETC2RGB {
			return ErrNativeHDR
		}

		for _, mat := range page.Materials {
			if mat.AlbedoEncoding == bc1Encoding && format != texture.BC1 ||
				mat.AlbedoEncoding == etc2Encoding && format != texture.ETC2RGB {
				return ErrNativeHDR
			}

			if mat.AlbedoEncoding == etc1sEncoding && (format == texture.RGBA8 || !decoderAvailable) {
				return basistranscode.ErrUnsupported
			}
		}
	}

	return nil
}
