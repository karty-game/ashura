package level

import (
	"errors"

	"github.com/karty-game/ashura/internal/basistranscode"
	"github.com/karty-game/ashura/pkg/texture"
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
func validateDeviceProfile(chunk Chunk, format texture.Format, supports func(texture.Format) bool, decoderAvailable bool) error {
	for _, page := range chunk.Pages {
		lightmap, _ := lightmapImageProfile(page)
		if !textureAvailable(lightmap, supports) {
			return ErrNativeTextures
		}

		for _, mat := range page.Materials {
			if mat.AlbedoEncoding == bc1Encoding && !textureAvailable(texture.BC1, supports) ||
				mat.AlbedoEncoding == etc2Encoding && !textureAvailable(texture.ETC2RGB, supports) ||
				mat.SSBump != nil && !textureAvailable(ssbumpFormat(page.Storage), supports) {
				return ErrNativeTextures
			}

			if mat.AlbedoEncoding == etc1sEncoding && (format == texture.RGBA8 || !decoderAvailable) {
				return basistranscode.ErrUnsupported
			}
		}
	}

	return nil
}

func textureAvailable(format texture.Format, supports func(texture.Format) bool) bool {
	return format == texture.RGBA8 || supports != nil && supports(format)
}
