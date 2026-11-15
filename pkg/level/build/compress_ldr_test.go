//go:build !js

package levelbuild

import (
	"errors"
	"testing"
	"time"

	"github.com/karty-game/karty-webgpu/internal/leveldata"
	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
	"github.com/karty-game/karty-webgpu/pkg/texture"
)

func ldrProfile(t *testing.T, family, format, transfer string) NativeTextureOptions {
	t.Helper()

	profile, err := normalizeNativeTextures(
		NativeTextureOptions{
			Family:         family,
			LightmapFormat: format,
			Transfer:       transfer,
			Encoder:        assetbuild.BlockCompressionOptions{},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	return profile
}
func TestNativeTextureDefaultsAndExactStorage(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		family, format, storage string
		target                  texture.Format
	}{
		{"", nativeTextureBC7, leveldata.StorageBC7LDR, texture.BC7},
		{nativeTextureETC2, nativeTextureETC2, leveldata.StorageETC2LDR, texture.ETC2RGB},
	} {
		profile := ldrProfile(t, test.family, "", "")
		if profile.LightmapFormat != test.format || profile.Transfer != nativeTextureSRGB || nativeTextureTarget(profile) != test.target ||
			nativeTextureStorage(profile) != test.storage ||
			nativeTextureSpace(profile) != texture.SRGB {
			t.Fatal("wrong normalized profile", profile)
		}
	}

	profile := ldrProfile(t, nativeTextureBC, nativeTextureBC1, nativeTextureLinear)
	if nativeTextureTarget(profile) != texture.BC1 || nativeTextureStorage(profile) != leveldata.StorageBC1LDR ||
		nativeTextureSpace(profile) != texture.Linear {
		t.Fatal(profile)
	}

	if publicationSchema(PublishOptions{NativeTextures: &profile}) != leveldata.Schema7 ||
		pagePublicationSchema(leveldata.Schema7, leveldata.Page{Representation: leveldata.RNMFit}) != leveldata.Schema7 {
		t.Fatal("LDR schema downgraded")
	}
}
func TestNativeTextureRejectsMismatchedOptionsBeforeActions(t *testing.T) {
	t.Parallel()
	base := ldrProfile(t, "", "", "")
	invalid := []NativeTextureOptions{base, base, base, base, base, base, base, base}
	invalid[0].Family = "etc2-astc"
	invalid[1].LightmapFormat = nativeTextureETC2
	invalid[2].Transfer = "gamma2.2"
	invalid[3].LightmapFormat = "unsupported"
	invalid[4].Transfer = "unsupported"
	invalid[5].Encoder.Timeout = -time.Second
	quality := maxNativeTextureQuality + 1
	invalid[6].Encoder.LDRQuality = &quality

	invalid[7].Encoder.SSBumpQuality = &quality
	for _, profile := range invalid {
		if _, err := normalizeNativeTextures(profile); !errors.Is(err, ErrInvalid) {
			t.Fatal("accepted invalid LDR profile", profile, err)
		}
	}

	for _, options := range []PublishOptions{
		{NativeHDR: &NativeHDROptions{}, NativeTextures: &base},
		{NativeTextures: &NativeTextureOptions{Encoder: base.Encoder, NativeAlbedo: true}},
	} {
		if err := Publish(options).Plan(&assetbuild.Plan{}); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid publication reached planning", err)
		}
	}
}
func TestNativeTextureLightmapPlanningSharesPreparationAcrossFamilies(t *testing.T) {
	t.Parallel()

	var plan assetbuild.Plan

	shared := &codecActionCache{}
	file := codecFixtureFile()
	desktop := ldrProfile(t, nativeTextureBC, nativeTextureBC7, nativeTextureSRGB)
	mobile := ldrProfile(t, nativeTextureETC2, nativeTextureETC2, nativeTextureSRGB)
	optional := ldrProfile(t, nativeTextureBC, nativeTextureBC1, nativeTextureSRGB)

	profiles := []NativeTextureOptions{desktop, mobile, desktop, optional}
	outputs := make([]assetbuild.Ref, 0, len(profiles))

	for _, profile := range profiles {
		ref, err := addLDRLightmapAction(&plan, "variant", "lightmap", file, profile, shared)
		if err != nil {
			t.Fatal(err)
		}

		outputs = append(outputs, ref)
	}

	if outputs[0] != outputs[2] || outputs[0] == outputs[1] || outputs[0] == outputs[3] || len(shared.refs) != 4 {
		t.Fatal("preparation or target sharing changed", outputs, len(shared.refs))
	}

	quality := 1

	desktop.Encoder.LDRQuality = &quality
	if _, err := addLDRLightmapAction(&plan, "quality", "lightmap", file, desktop, shared); err != nil {
		t.Fatal(err)
	}

	if len(shared.refs) != 6 {
		t.Fatal("quality change reused stale prepared blocks", len(shared.refs))
	}
}
func TestNativeTextureSSBumpRolesKeepPortableDataAndShareDesktopCodec(t *testing.T) {
	t.Parallel()

	var plan assetbuild.Plan

	shared := &codecActionCache{}
	file := codecFixtureFile()

	families := []string{nativeTextureBC, nativeTextureETC2, nativeTextureBC}
	outputs := make([]assetbuild.Ref, 0, len(families))

	for _, family := range families {
		profile := ldrProfile(t, family, "", "")
		input := nativeBlockInputs{files: map[string]fileRecord{"ssbump/source": file}, lightmaps: map[string]bool{}}

		encoded, _, err := nativeTextureActions(&plan, "variant", input, profile, shared)
		if err != nil {
			t.Fatal(err)
		}

		outputs = append(outputs, encoded["ssbump/source"])
	}

	if outputs[0] != outputs[2] || outputs[0] == outputs[1] || len(shared.refs) != 3 {
		t.Fatal("desktop codec or portable authored-data action duplicated", outputs, len(shared.refs))
	}

	profile := ldrProfile(t, nativeTextureETC2, "", "")
	quality := maxNativeTextureQuality
	profile.Encoder.SSBumpQuality = &quality

	encoded, _, err := nativeTextureActions(&plan, "same-portable-data", nativeBlockInputs{
		files: map[string]fileRecord{"ssbump/source": file}, lightmaps: map[string]bool{},
	}, profile, shared)
	if err != nil || encoded["ssbump/source"] != outputs[1] || len(shared.refs) != 3 {
		t.Fatal("portable raw coefficients depend on unused codec options", err, encoded, len(shared.refs))
	}

	adapter := nativeTextureAlbedoProfile(profile)
	if adapter.Family != nativeHDRMobileFamily {
		t.Fatal("portable albedo adapter selected the wrong family", adapter)
	}
}

func TestLDRNativeAlbedoDoesNotAliasLegacyContainerProducer(t *testing.T) {
	t.Parallel()

	var plan assetbuild.Plan

	shared := &codecActionCache{}
	profile := nativeTextureAlbedoProfile(ldrProfile(t, nativeTextureBC, "", ""))
	files := map[string]fileRecord{"albedo": codecFixtureFile()}

	old, _, err := nativeAlbedoActions(&plan, "legacy", files, profile, shared)
	if err != nil {
		t.Fatal(err)
	}

	current, _, err := nativeLDRAlbedoActions(&plan, "ldr", files, profile, shared)
	if err != nil {
		t.Fatal(err)
	}

	repeated, _, err := nativeLDRAlbedoActions(&plan, "repeat", files, profile, shared)
	if err != nil {
		t.Fatal(err)
	}

	if old["albedo"] == current["albedo"] || current["albedo"] != repeated["albedo"] {
		t.Fatal("legacy and canonical mip containers shared an encoder identity", old, current, repeated)
	}
}
