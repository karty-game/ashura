//go:build linux

package native

import (
	"errors"
	"slices"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/karty-game/ashura/internal/texturedata"
	wgpu "github.com/karty-game/ashura/internal/webgpu"
)

type testCompressionFeatures map[wgpu.FeatureName]bool

func (features testCompressionFeatures) HasFeature(feature wgpu.FeatureName) bool {
	return features[feature]
}

func TestNativeCompressionNegotiation(t *testing.T) {
	t.Parallel()

	features := testCompressionFeatures{
		wgpu.FeatureNameTextureCompressionBC: true, wgpu.FeatureNameTextureCompressionETC2: true,
	}
	if got, want := compressionFeatures(features), []wgpu.FeatureName{
		wgpu.FeatureNameTextureCompressionBC, wgpu.FeatureNameTextureCompressionETC2,
	}; !slices.Equal(got, want) {
		t.Fatalf("requested features %v, want %v", got, want)
	}

	// The device can expose fewer features than the adapter. Selection must use
	// the enabled snapshot, and must not query native capabilities during draws.
	features[wgpu.FeatureNameTextureCompressionBC] = false
	driver := Driver{compression: enabledCompression(features)}
	features[wgpu.FeatureNameTextureCompressionETC2] = false

	if got := driver.CompressedAlbedoFormat(); got != texturedata.ETC2RGB {
		t.Fatalf("enabled ETC2 device selected %v", got)
	}

	driver.compression.bc = true
	if got := driver.CompressedAlbedoFormat(); got != texturedata.BC1 {
		t.Fatalf("BC-capable device selected %v", got)
	}

	driver.compression = nativeCompression{astc: true}
	if got := driver.CompressedAlbedoFormat(); got != texturedata.RGBA8 {
		t.Fatalf("device without a supported level profile selected %v", got)
	}

	if got := compressionFeatures(testCompressionFeatures{}); len(got) != 0 {
		t.Fatalf("requested unsupported features %v", got)
	}
}

func TestNativePreparedTextureFormats(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		encoding texturedata.Format
		feature  wgpu.FeatureName
		linear   gputypes.TextureFormat
		srgb     gputypes.TextureFormat
	}{
		{"RGBA8", texturedata.RGBA8, 0, gputypes.TextureFormatRGBA8Unorm, gputypes.TextureFormatRGBA8UnormSrgb},
		{"BC1", texturedata.BC1, wgpu.FeatureNameTextureCompressionBC,
			gputypes.TextureFormatBC1RGBAUnorm, gputypes.TextureFormatBC1RGBAUnormSrgb},
		{"BC6H", texturedata.BC6H, wgpu.FeatureNameTextureCompressionBC, gputypes.TextureFormatBC6HRGBUfloat, 0},
		{"BC7", texturedata.BC7, wgpu.FeatureNameTextureCompressionBC,
			gputypes.TextureFormatBC7RGBAUnorm, gputypes.TextureFormatBC7RGBAUnormSrgb},
		{"ETC2RGB", texturedata.ETC2RGB, wgpu.FeatureNameTextureCompressionETC2,
			gputypes.TextureFormatETC2RGB8Unorm, gputypes.TextureFormatETC2RGB8UnormSrgb},
		{"ETC2RGBA", texturedata.ETC2RGBA, wgpu.FeatureNameTextureCompressionETC2,
			gputypes.TextureFormatETC2RGBA8Unorm, gputypes.TextureFormatETC2RGBA8UnormSrgb},
		{"ASTC4x4", texturedata.ASTC4x4, wgpu.FeatureNameTextureCompressionASTC,
			gputypes.TextureFormatASTC4x4Unorm, gputypes.TextureFormatASTC4x4UnormSrgb},
		{"ASTC6x6", texturedata.ASTC6x6, wgpu.FeatureNameTextureCompressionASTC,
			gputypes.TextureFormatASTC6x6Unorm, gputypes.TextureFormatASTC6x6UnormSrgb},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			features := testCompressionFeatures{test.feature: true}
			driver := Driver{compression: enabledCompression(features)}

			for _, colorSpace := range []texturedata.ColorSpace{texturedata.Linear, texturedata.SRGB} {
				want := test.linear
				if colorSpace == texturedata.SRGB {
					want = test.srgb
				}

				if want == 0 {
					continue // BC6H has no sRGB representation; texture admission rejects it.
				}

				image := nativePreparedTexture(t, test.encoding, colorSpace)

				got, err := driver.nativeTextureFormat(image)
				if err != nil || got != want {
					t.Fatalf("color space %v: format %v, error %v, want %v", colorSpace, got, err, want)
				}

				if test.encoding != texturedata.RGBA8 {
					assertUnsupportedNativeTexture(t, image, test.feature)
				}
			}
		})
	}
}

func assertUnsupportedNativeTexture(t *testing.T, image *texturedata.Texture, missing wgpu.FeatureName) {
	t.Helper()

	// Every other compression feature is enabled: the exact required feature
	// must still be present before any residency mutation.
	unsupported := Driver{compression: enabledCompression(testCompressionFeatures{
		wgpu.FeatureNameTextureCompressionBC:   missing != wgpu.FeatureNameTextureCompressionBC,
		wgpu.FeatureNameTextureCompressionETC2: missing != wgpu.FeatureNameTextureCompressionETC2,
		wgpu.FeatureNameTextureCompressionASTC: missing != wgpu.FeatureNameTextureCompressionASTC,
	})}
	if err := unsupported.PrepareTexture(image); !errors.Is(err, errNativeCompression) {
		t.Fatalf("unsupported device returned %v", err)
	}

	if unsupported.textures != nil || unsupported.textureBytes != 0 {
		t.Fatal("unsupported format changed GPU residency")
	}
}

func TestNativeCompressedPreparationReusesCache(t *testing.T) {
	t.Parallel()

	image := nativePreparedTexture(t, texturedata.BC1, texturedata.SRGB)
	bytes := texturedata.ByteSize(image)
	// A nil device proves a repeated preparation neither queries GPU features
	// nor allocates or uploads another compressed image.
	driver := Driver{
		compression: nativeCompression{bc: true},
		textures:    map[*texturedata.Texture]gpuTexture{image: {bytes: bytes}}, textureBytes: bytes,
	}
	for range 2 {
		if err := driver.PrepareTexture(image); err != nil {
			t.Fatal(err)
		}
	}

	if len(driver.textures) != 1 || driver.textureBytes != bytes {
		t.Fatal("cached compressed preparation changed texture residency")
	}
}

func nativePreparedTexture(t *testing.T, encoding texturedata.Format, colorSpace texturedata.ColorSpace) *texturedata.Texture {
	t.Helper()

	side := texturedata.BlockDimension(encoding)
	rowBytes, rows := texturedata.Layout(encoding, side, side)

	image, err := texturedata.FromOwned(texturedata.Data{
		Width: side, Height: side, Format: encoding, ColorSpace: colorSpace, Levels: [][]byte{make([]byte, rowBytes*rows)},
	})
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { texturedata.Release(image) })

	return image
}
