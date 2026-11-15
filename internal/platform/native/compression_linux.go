//go:build linux

package native

import (
	"errors"

	"github.com/gogpu/gputypes"
	"github.com/karty-game/karty-webgpu/internal/texturedata"
	wgpu "github.com/karty-game/karty-webgpu/internal/webgpu"
)

var errNativeCompression = errors.New("compressed texture format is not enabled on this native device")

type featureProvider interface {
	HasFeature(feature wgpu.FeatureName) bool
}

type nativeCompression struct {
	bc, etc2, astc bool
}

// compressionFeatures requests only formats the selected adapter can enable.
func compressionFeatures(adapter featureProvider) []wgpu.FeatureName {
	var required []wgpu.FeatureName

	for _, feature := range [...]wgpu.FeatureName{
		wgpu.FeatureNameTextureCompressionBC,
		wgpu.FeatureNameTextureCompressionETC2,
		wgpu.FeatureNameTextureCompressionASTC,
	} {
		if adapter.HasFeature(feature) {
			required = append(required, feature)
		}
	}

	return required
}

// enabledCompression snapshots device features, rather than adapter promises.
func enabledCompression(device featureProvider) nativeCompression {
	return nativeCompression{
		bc:   device.HasFeature(wgpu.FeatureNameTextureCompressionBC),
		etc2: device.HasFeature(wgpu.FeatureNameTextureCompressionETC2),
		astc: device.HasFeature(wgpu.FeatureNameTextureCompressionASTC),
	}
}

// CompressedAlbedoFormat selects prepared level payloads for the enabled device.
func (driver *Driver) CompressedAlbedoFormat() texturedata.Format {
	if driver.compression.bc {
		return texturedata.BC1
	}

	if driver.compression.etc2 {
		return texturedata.ETC2RGB
	}

	return texturedata.RGBA8
}

func (driver *Driver) nativeTextureFormat(source *texturedata.Texture) (gputypes.TextureFormat, error) {
	enabled, linear, srgb := driver.compression.formats(texturedata.Encoding(source))
	if !enabled {
		return 0, errNativeCompression
	}

	if texturedata.Space(source) == texturedata.SRGB {
		if srgb == 0 {
			return 0, errNativeCompression
		}

		return srgb, nil
	}

	return linear, nil
}

func (compression nativeCompression) formats(encoding texturedata.Format) (bool, gputypes.TextureFormat, gputypes.TextureFormat) {
	switch encoding {
	case texturedata.RGBA8:
		return true, gputypes.TextureFormatRGBA8Unorm, gputypes.TextureFormatRGBA8UnormSrgb
	case texturedata.BC1:
		return compression.bc, gputypes.TextureFormatBC1RGBAUnorm, gputypes.TextureFormatBC1RGBAUnormSrgb
	case texturedata.BC6H:
		return compression.bc, gputypes.TextureFormatBC6HRGBUfloat, 0
	case texturedata.BC7:
		return compression.bc, gputypes.TextureFormatBC7RGBAUnorm, gputypes.TextureFormatBC7RGBAUnormSrgb
	case texturedata.ETC2RGB:
		return compression.etc2, gputypes.TextureFormatETC2RGB8Unorm, gputypes.TextureFormatETC2RGB8UnormSrgb
	case texturedata.ETC2RGBA:
		return compression.etc2, gputypes.TextureFormatETC2RGBA8Unorm, gputypes.TextureFormatETC2RGBA8UnormSrgb
	case texturedata.ASTC4x4:
		return compression.astc, gputypes.TextureFormatASTC4x4Unorm, gputypes.TextureFormatASTC4x4UnormSrgb
	}

	return false, 0, 0
}
