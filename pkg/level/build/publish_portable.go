//go:build !js

package levelbuild

import "github.com/karty-game/karty-webgpu/pkg/assetbuild"

const portableAlbedoQuality = 192

// PortableOptions selects normalized LDR RGB/RNM delivery for BC and ETC2
// devices. Embedded portable codecs run offline; games load GPU blocks.
type PortableOptions struct {
	MaxAtlasSize uint32
	Encoder      assetbuild.BlockCompressionOptions
}

// PublishPortable emits RGBBC, RGBMobile, RNMBC and RNMMobile descriptors from
// one RGB+RNM bake. Albedos use offline ETC1S quality192; LDR lightmaps use
// quality0 and desktop relief quality0 unless explicitly supplied in Encoder.
func PublishPortable(options PortableOptions) assetbuild.Plugin {
	encoder := options.Encoder

	if encoder.LDRQuality == nil {
		quality := 0
		encoder.LDRQuality = &quality
	}

	if encoder.SSBumpQuality == nil {
		quality := 0
		encoder.SSBumpQuality = &quality
	}

	profile := func(name, family, representation string) PublishOptions {
		return PublishOptions{Variant: name, Representation: representation, MaxAtlasSize: options.MaxAtlasSize,
			NativeTextures: &NativeTextureOptions{Family: family, NativeAlbedo: true, Encoder: encoder},
			AlbedoETC1S:    &ETC1SOptions{Quality: portableAlbedoQuality},
		}
	}

	return PublishVariants(profile("RGBBC", "bc", "rgb-diffuse-v1"), profile("RGBMobile", "etc2", "rgb-diffuse-v1"),
		profile("RNMBC", "bc", "rnm-fit-v1"), profile("RNMMobile", "etc2", "rnm-fit-v1"))
}
