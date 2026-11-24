//go:build !js

package assetbuild

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/karty-game/ashura/internal/ktx2data"
	"github.com/karty-game/ashura/pkg/texture"
)

//nolint:paralleltest // Serial codec qualification bounds simultaneous VM memory.
func TestKTX2ExplicitFormatsPreserveRolesAndMips(t *testing.T) {
	ctx, closeCodec := buildCodecContext(t.Context())
	defer func() { _ = closeCodec() }()

	set := ImageSet{}

	for _, usage := range []string{UsageAlbedo, UsageNormal, UsageSSBump, UsageAO} {
		value := atlasTestImage(AssetName(usage), usage, 4, 4, [4]byte{128, 128, 255, 255})
		value.Source = usage + ".png"

		value, err := GenerateMipmaps(value)
		if err != nil {
			t.Fatal(err)
		}

		set.Images = append(set.Images, value)
	}

	options := KTX2Options{UsageFormats: map[string]texture.Format{
		UsageAlbedo: texture.BC1, UsageNormal: texture.BC7, UsageSSBump: texture.BC7,
	}}
	checkKTX2RoleFormats(t, ctx, set, options)
	options.UsageFormats = map[string]texture.Format{
		UsageAlbedo: texture.ETC2RGB, UsageNormal: texture.ETC2RGBA, UsageSSBump: texture.ASTC4x4,
	}
	checkKTX2RoleFormats(t, ctx, set, options)
}

func checkKTX2RoleFormats(t *testing.T, ctx context.Context, set ImageSet, options KTX2Options) {
	t.Helper()

	result, err := prepareImageSet(ctx, set, options, nil)
	if err != nil {
		t.Fatal(err)
	}

	var prepared PreparedSet
	if err := json.Unmarshal(result[imageDataSlot], &prepared); err != nil {
		t.Fatal(err)
	}

	for index, image := range prepared.Images {
		decoded, err := ktx2data.Decode(image.Data)
		if err != nil || decoded.Format != options.UsageFormats[image.Usage] || decoded.Width != 4 || decoded.Height != 4 ||
			len(decoded.Levels) != len(set.Images[index].Levels) {
			t.Fatal("KTX2 role storage changed dimensions or mips", image.Usage, decoded, err)
		}

		if (decoded.ColorSpace == texture.SRGB) != (image.Usage == UsageAlbedo) {
			t.Fatal("KTX2 role transfer", image.Usage)
		}
	}
}

func TestKTX2FormatOptionsRejectUnsupportedSelections(t *testing.T) {
	t.Parallel()

	for _, options := range [][]KTX2Options{
		{{}, {}}, {{Format: texture.BC6H}}, {{UsageFormats: map[string]texture.Format{"unknown": texture.BC1}}},
		{{UsageFormats: map[string]texture.Format{UsageHeight: texture.BC7}}},
	} {
		if _, err := normalizedKTX2Options(options); err == nil {
			t.Fatal("unsupported KTX2 selection accepted", options)
		}
	}
}

//nolint:paralleltest // Serial codec qualification bounds simultaneous VM memory.
func TestKTX2VariantsShareCommonPayloadsAndCatalog(t *testing.T) {
	ctx, closeCodec := buildCodecContext(t.Context())
	defer func() { _ = closeCodec() }()

	albedo := atlasTestImage("Albedo", UsageAlbedo, 4, 4, [4]byte{128, 128, 255, 255})
	albedo.Source = "albedo.png"
	ao := atlasTestImage("Occlusion", UsageAO, 4, 4, [4]byte{128, 128, 128, 255})
	ao.Source = "occlusion.png"
	set := ImageSet{Images: []Image{albedo, ao}}
	primary := KTX2Options{UsageFormats: map[string]texture.Format{UsageAlbedo: texture.BC1}}
	mobile := KTX2Options{UsageFormats: map[string]texture.Format{UsageAlbedo: texture.ETC2RGB}}

	output, err := prepareImageSet(ctx, set, primary, &mobile)
	if err != nil {
		t.Fatal(err)
	}

	var primaryImages PreparedSet
	if err := json.Unmarshal(output[imageDataSlot], &primaryImages); err != nil {
		t.Fatal(err)
	}

	if len(primaryImages.Images) != len(set.Images) || primaryImages.Images[0].Output != "albedo.ktx2" {
		t.Fatal("prepared inputs should contain one primary image per source", primaryImages.Images)
	}

	var files Bundle
	if err := json.Unmarshal(output[imageFilesSlot], &files); err != nil {
		t.Fatal(err)
	}

	want := map[string]texture.Format{"albedo.ktx2": texture.BC1, "mobile/albedo.ktx2": texture.ETC2RGB, "occlusion.ktx2": texture.RGBA8}
	if len(files) != len(want) {
		t.Fatal("unexpected variant file count", len(files))
	}

	for path, format := range want {
		decoded, err := ktx2data.Decode(files[path])
		if err != nil || decoded.Format != format {
			t.Fatal("unexpected format", path, decoded.Format, err)
		}
	}

	var metadata PreparedSet
	if err := json.Unmarshal(output[imageCatalogSlot], &metadata); err != nil {
		t.Fatal(err)
	}

	code, err := catalogCode(metadata)
	if err != nil || !strings.Contains(string(code), "func AlbedoMobile()") || strings.Contains(string(code), "func OcclusionMobile()") {
		t.Fatal("unexpected variant catalog", string(code), err)
	}
}
