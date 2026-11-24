//go:build !js

package levelbuild

import (
	"testing"

	"github.com/karty-game/ashura/pkg/assetbuild"
)

func TestPortableHDREffortBounds(t *testing.T) {
	t.Parallel()

	profile := NativeHDROptions{Family: nativeHDRBCFamily, Encoder: assetbuild.BlockCompressionOptions{
		HDREffort: 1,
	}}
	if err := validateNativeHDR(profile); err != nil {
		t.Fatal("valid portable HDR effort rejected", err)
	}

	profile.Encoder.HDREffort = maxNativeHDREffort + 1
	if err := validateNativeHDR(profile); err == nil {
		t.Fatal("native HDR effort exceeded the pinned codec profile")
	}
}

func TestSSBumpPreparationIdentityKeepsOnlyRelevantQuality(t *testing.T) {
	t.Parallel()

	ssbump, rgbe := 2, 1

	encoder := ssbumpEncoder(NativeHDROptions{Encoder: assetbuild.BlockCompressionOptions{
		HDREffort: 1, SSBumpQuality: &ssbump, RGBEQuality: &rgbe,
	}})
	if encoder.SSBumpQuality == nil || *encoder.SSBumpQuality != ssbump || encoder.RGBEQuality != nil {
		t.Fatal("shared SSBump stage lost its effort or retained unrelated HDR settings", encoder)
	}

	invalid := maxNativeHDREffort + 1

	profile := NativeHDROptions{Family: nativeHDRMobileFamily, Encoder: assetbuild.BlockCompressionOptions{
		SSBumpQuality: &invalid,
	}}
	if err := validateNativeHDR(profile); err == nil {
		t.Fatal("invalid explicit coefficient effort entered the asset plan")
	}
}
