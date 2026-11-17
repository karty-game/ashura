package ktx2data_test

import (
	_ "embed"
	"encoding/binary"
	"testing"

	"github.com/karty-game/karty-webgpu/internal/ktx2data"
	"github.com/karty-game/karty-webgpu/internal/texturedata"
)

//go:embed testdata/bc6h.ktx2
var bc6h []byte

//go:embed testdata/bc7.ktx2
var bc7 []byte

//go:embed testdata/bc1-srgb.ktx2
var bc1SRGB []byte

//go:embed testdata/etc2rgb-srgb.ktx2
var etc2RGBSRGB []byte

//go:embed testdata/etc2rgb-linear.ktx2
var etc2RGBLinear []byte

//go:embed testdata/etc2rgba-linear.ktx2
var etc2RGBALinear []byte

//go:embed testdata/astc4x4-linear.ktx2
var astc4x4Linear []byte

func TestNativeBlocksAdmitWithoutDecoding(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name   string
		data   []byte
		format texturedata.Format
		levels int
		space  texturedata.ColorSpace
	}{
		{"BC6H", bc6h, texturedata.BC6H, 1, texturedata.Linear},
		{"BC7", bc7, texturedata.BC7, 4, texturedata.Linear},
		{"BC7 sRGB", bc7SRGBFixture(), texturedata.BC7, 4, texturedata.SRGB},
		{"BC1 sRGB", bc1SRGB, texturedata.BC1, 4, texturedata.SRGB},
		{"ETC2 RGB sRGB", etc2RGBSRGB, texturedata.ETC2RGB, 4, texturedata.SRGB},
		{"ETC2 RGB linear", etc2RGBLinear, texturedata.ETC2RGB, 4, texturedata.Linear},
		{"ETC2 RGBA linear", etc2RGBALinear, texturedata.ETC2RGBA, 4, texturedata.Linear},
		{"ETC2 RGBA sRGB", etc2RGBASRGBFixture(), texturedata.ETC2RGBA, 4, texturedata.SRGB},
		{"ASTC4x4 linear", astc4x4Linear, texturedata.ASTC4x4, 4, texturedata.Linear},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			image, err := ktx2data.Decode(test.data)
			if err != nil {
				t.Fatal(err)
			}

			if image.Width != 8 || image.Height != 8 || image.Format != test.format || image.ColorSpace != test.space ||
				len(image.Levels) != test.levels {
				t.Fatal(image)
			}

			offset := binary.LittleEndian.Uint64(test.data[80:])
			if &image.Levels[0][0] != &test.data[offset] {
				t.Fatal("admission copied block bytes")
			}

			if _, err = texturedata.FromOwned(image); err != nil {
				t.Fatal(err)
			}

			for length := range len(test.data) {
				if _, err := ktx2data.Decode(test.data[:length]); err == nil {
					t.Fatalf("accepted truncated block container %d", length)
				}
			}

			for _, mutate := range []func([]byte){
				func(b []byte) { binary.LittleEndian.PutUint32(b[44:], 1) },
				func(b []byte) { binary.LittleEndian.PutUint32(b[20:], 7) },
				func(b []byte) { binary.LittleEndian.PutUint64(b[88:], 65) },
				func(b []byte) { offset := binary.LittleEndian.Uint32(b[48:]); b[offset+14] ^= 3 },
				func(b []byte) { offset := binary.LittleEndian.Uint32(b[48:]); b[offset+20] = 0 },
				func(b []byte) { offset := binary.LittleEndian.Uint32(b[48:]); b[offset+31] ^= 16 },
			} {
				damaged := append([]byte(nil), test.data...)
				mutate(damaged)

				if _, err := ktx2data.Decode(damaged); err == nil {
					t.Fatal("accepted malformed native block metadata")
				}
			}
		})
	}
}

func bc7SRGBFixture() []byte {
	data := append([]byte(nil), bc7...)
	// Vulkan format146 has the same BC7 blocks with an sRGB transfer descriptor.
	binary.LittleEndian.PutUint32(data[12:], 146)
	descriptor := binary.LittleEndian.Uint32(data[48:])
	data[descriptor+14] = 2

	return data
}

func etc2RGBASRGBFixture() []byte {
	data := append([]byte(nil), etc2RGBALinear...)
	binary.LittleEndian.PutUint32(data[12:], 152)

	descriptor := binary.LittleEndian.Uint32(data[48:])
	data[descriptor+14] = 2
	data[descriptor+31] = 31 // Alpha stays linear with sRGB RGB transfer.

	return data
}

func TestETC2RGBARequiresBothChannelDescriptors(t *testing.T) {
	t.Parallel()

	for _, mutate := range []func([]byte){
		func(data []byte) { binary.LittleEndian.PutUint32(data[52:], 44) },
		func(data []byte) { offset := binary.LittleEndian.Uint32(data[48:]); data[offset+31] = 2 },
		func(data []byte) { offset := binary.LittleEndian.Uint32(data[48:]); data[offset+47] = 15 },
		func(data []byte) {
			offset := binary.LittleEndian.Uint32(data[48:])
			binary.LittleEndian.PutUint16(data[offset+44:], 0)
		},
	} {
		data := append([]byte(nil), etc2RGBALinear...)
		mutate(data)

		if _, err := ktx2data.Decode(data); err == nil {
			t.Fatal("accepted invalid ETC2 alpha/RGB channel descriptors")
		}
	}
}

func TestBC1RGBLinearColorSpace(t *testing.T) {
	t.Parallel()

	// Native BC1 RGB bytes are unchanged between linear/sRGB containers. Both
	// the Vulkan format and matching transfer descriptor must change together.
	data := append([]byte(nil), bc1SRGB...)
	binary.LittleEndian.PutUint32(data[12:], 131)
	offset := binary.LittleEndian.Uint32(data[48:])
	data[offset+14] = 1

	image, err := ktx2data.Decode(data)
	if err != nil || image.Format != texturedata.BC1 || image.ColorSpace != texturedata.Linear {
		t.Fatal("linear BC1 RGB admission", image, err)
	}
}
