//go:build !js

package assetbuild

import (
	"encoding/json"
	"testing"

	"github.com/karty-game/karty-webgpu/internal/ktx2data"
	"github.com/karty-game/karty-webgpu/pkg/texture"
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

	result, err := prepareImageSet(ctx, set, options)
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
