//go:build !js

package levelbuild

import (
	"fmt"

	"github.com/karty-game/karty-webgpu/internal/leveldata"
	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
	"github.com/karty-game/karty-webgpu/pkg/texture"
)

// NativeTextureOptions publishes normalized eight-bit lightmaps as GPU blocks.
// Floating-point solving and the shared RGB/RNM tuple scale remain unchanged.
// Only LDRQuality, SSBumpQuality and Timeout are consumed from Encoder;
// HDR settings have no role in this delivery profile.
type NativeTextureOptions struct {
	Encoder assetbuild.BlockCompressionOptions

	// Family selects "bc" (the default) or "etc2". ETC2 uses portable
	// compressed lightmaps/albedos and exact linear RGBA8 SSBump with authored mips.
	Family string
	// LightmapFormat selects "bc7" (BC default), optional "bc1", or "etc2".
	LightmapFormat string
	// Transfer selects "srgb" (the default) or "linear" for stored RGB bytes.
	// Both use the existing linear sampled-value shader path and shared scale.
	Transfer string
	// NativeAlbedo prepares opaque albedos offline through AlbedoETC1S's input
	// preparation, retaining BC1/ETC2 blocks rather than a runtime decoder path.
	NativeAlbedo bool
}

const (
	nativeTextureBC         = "bc"
	nativeTextureETC2       = "etc2"
	nativeTextureSRGB       = leveldata.EncodingSRGB
	nativeTextureLinear     = "linear"
	nativeTextureBC7        = "bc7"
	nativeTextureBC1        = "bc1"
	maxNativeTextureQuality = 4
)

func normalizeNativeTextures(profile NativeTextureOptions) (NativeTextureOptions, error) {
	if profile.Family == "" {
		profile.Family = nativeTextureBC
	}

	if profile.LightmapFormat == "" {
		profile.LightmapFormat = nativeTextureBC7
		if profile.Family == nativeTextureETC2 {
			profile.LightmapFormat = nativeTextureETC2
		}
	}

	if profile.Transfer == "" {
		profile.Transfer = nativeTextureSRGB
	}

	if profile.Family != nativeTextureBC && profile.Family != nativeTextureETC2 ||
		profile.Transfer != nativeTextureSRGB && profile.Transfer != nativeTextureLinear {
		return NativeTextureOptions{}, fmt.Errorf("invalid native LDR family/transfer: %w", ErrInvalid)
	}

	if profile.Family == nativeTextureBC && profile.LightmapFormat != nativeTextureBC7 && profile.LightmapFormat != nativeTextureBC1 ||
		profile.Family == nativeTextureETC2 && profile.LightmapFormat != nativeTextureETC2 {
		return NativeTextureOptions{}, fmt.Errorf("native LDR format/family mismatch: %w", ErrInvalid)
	}

	encoder, err := normalizeLDRTextureEncoder(profile.Encoder)
	if err != nil {
		return NativeTextureOptions{}, err
	}

	profile.Encoder = encoder

	return profile, nil
}

func normalizeLDRTextureEncoder(encoder assetbuild.BlockCompressionOptions) (assetbuild.BlockCompressionOptions, error) {
	if encoder.Timeout < 0 {
		return assetbuild.BlockCompressionOptions{}, fmt.Errorf("invalid native LDR encoder: %w", ErrInvalid)
	}

	for _, quality := range []*int{encoder.LDRQuality, encoder.SSBumpQuality} {
		if quality != nil && (*quality < 0 || *quality > maxNativeTextureQuality) {
			return assetbuild.BlockCompressionOptions{}, fmt.Errorf("invalid native LDR quality: %w", ErrInvalid)
		}
	}

	quality := 0
	if encoder.LDRQuality != nil {
		quality = *encoder.LDRQuality
	}

	return assetbuild.BlockCompressionOptions{

		Timeout:       encoder.Timeout,
		LDRQuality:    &quality,
		SSBumpQuality: encoder.SSBumpQuality,
	}, nil
}

func nativeLightmapEncoder(profile NativeTextureOptions) assetbuild.BlockCompressionOptions {
	encoder := profile.Encoder

	return assetbuild.BlockCompressionOptions{

		Timeout:    encoder.Timeout,
		LDRQuality: encoder.LDRQuality,
	}
}

func nativeTextureTarget(profile NativeTextureOptions) texture.Format {
	switch profile.LightmapFormat {
	case nativeTextureBC1:
		return texture.BC1
	case nativeTextureETC2:
		return texture.ETC2RGB
	default:
		return texture.BC7
	}
}
func nativeTextureSpace(profile NativeTextureOptions) texture.ColorSpace {
	if profile.Transfer == nativeTextureSRGB {
		return texture.SRGB
	}

	return texture.Linear
}
func nativeTextureStorage(profile NativeTextureOptions) string {
	switch profile.LightmapFormat {
	case nativeTextureBC1:
		return leveldata.StorageBC1LDR
	case nativeTextureETC2:
		return leveldata.StorageETC2LDR
	default:
		return leveldata.StorageBC7LDR
	}
}

// Internal adapter reuses the existing native-albedo producers without exposing
// HDR delivery semantics or requiring any HDR tool on the new profile path.
func nativeTextureAlbedoProfile(profile NativeTextureOptions) NativeHDROptions {
	family := nativeHDRBCFamily
	if profile.Family == nativeTextureETC2 {
		family = nativeHDRMobileFamily
	}

	return NativeHDROptions{Family: family, Encoder: profile.Encoder, NativeAlbedo: profile.NativeAlbedo}
}

type invalidNativePublisher struct{ err error }

func (invalidNativePublisher) Name() string                          { return levelPublishPluginName }
func (publisher invalidNativePublisher) Plan(*assetbuild.Plan) error { return publisher.err }
