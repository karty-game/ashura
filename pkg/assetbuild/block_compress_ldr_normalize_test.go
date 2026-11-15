//go:build !js

package assetbuild

import (
	"bytes"
	"image"
	"math"
	"testing"

	"github.com/karty-game/karty-webgpu/internal/ktx2data"
	"github.com/karty-game/karty-webgpu/pkg/texture"
)

func TestLDRCoefficientFamilySharesPhysicalScaleAndPreservesSource(t *testing.T) {
	t.Parallel()

	sources := [][]byte{
		ldrTestTexture(4, 4, [4]byte{10, 2, 0, 130}, texture.Linear),
		ldrTestTexture(4, 4, [4]byte{255, 128, 64, 128}, texture.Linear),
		ldrTestTexture(4, 4, [4]byte{64, 0, 0, 129}, texture.Linear),
	}

	before := make([][]byte, len(sources))
	for index := range sources {
		before[index] = bytes.Clone(sources[index])
	}

	output, scale, err := PrepareLDRLightmapCoefficients(sources, 3.5, texture.Linear)
	if err != nil || scale != 3.5 || len(output) != 3 {
		t.Fatal("wrong shared physical maximum", scale, len(output), err)
	}

	for index, data := range output {
		decoded, decodeErr := ktx2data.Decode(data)
		if decodeErr != nil {
			t.Fatal(decodeErr)
		}

		if decoded.ColorSpace != texture.Linear || decoded.Format != texture.RGBA8 || decoded.Width != 4 || decoded.Height != 4 ||
			len(decoded.Levels) != 1 {
			t.Fatal("wrong LDR image", decoded)
		}

		original, _ := ktx2data.Decode(sources[index])
		for c := range 3 {
			expected := decodeBlockRGBE(original.Levels[0], c) * 3.5

			actual := float32(decoded.Levels[0][c]) / 255 * scale
			if math.Abs(float64(actual-expected)) > float64(scale)/510+1e-6 {
				t.Fatal("physical radiance changed beyond LDR rounding", index, c, actual, expected)
			}
		}

		if decoded.Levels[0][3] != 255 {
			t.Fatal("RGBE exponent retained as alpha")
		}

		if !bytes.Equal(before[index], sources[index]) {
			t.Fatal("source coefficient mutated")
		}
	}
}

func TestLDRCoefficientSRGBTransferAndRGBEDivisor(t *testing.T) {
	t.Parallel()

	source := ldrTestTexture(1, 1, [4]byte{255, 128, 64, 128}, texture.Linear)

	output, scale, err := PrepareLDRLightmapCoefficients([][]byte{source}, 2, texture.SRGB)
	if err != nil || scale != 2 {
		t.Fatal("RGBE /255 physical maximum changed", scale, err)
	}

	decoded, err := ktx2data.Decode(output[0])
	if err != nil {
		t.Fatal(err)
	}

	if decoded.ColorSpace != texture.SRGB || !bytes.Equal(decoded.Levels[0], []byte{255, 188, 137, 255}) {
		t.Fatal("incorrect offline sRGB transfer", decoded)
	}
}

func TestLDRCoefficientLookupMatchesRGBEContract(t *testing.T) {
	t.Parallel()

	pixels := make([]byte, 256*256*4)

	for exponent := range 256 {
		for mantissa := range 256 {
			pixelOffset := (exponent*256 + mantissa) * 4
			pixels[pixelOffset], pixels[pixelOffset+1], pixels[pixelOffset+2], pixels[pixelOffset+3] = byte(
				mantissa,
			), byte(
				255-mantissa,
			), byte(
				mantissa,
			), byte(
				exponent,
			)
		}
	}

	source := encodeKTX([]*image.NRGBA{{Pix: pixels, Stride: 256 * 4, Rect: image.Rect(0, 0, 256, 256)}}, false)

	output, scale, err := PrepareLDRLightmapCoefficients([][]byte{source}, float32(math.Ldexp(1, -127)), texture.Linear)
	if err != nil || scale != 1 {
		t.Fatal(scale, err)
	}

	decoded, err := ktx2data.Decode(output[0])
	if err != nil {
		t.Fatal(err)
	}

	maximum := float64(float32(math.Ldexp(1, 127)))

	for pixelOffset := 0; pixelOffset < len(pixels); pixelOffset += 4 {
		for c := range 3 {
			want := byte(math.Round(float64(decodeBlockRGBE(pixels[pixelOffset:], c)) / maximum * 255))
			if decoded.Levels[0][pixelOffset+c] != want {
				t.Fatal("lookup differs from /255 RGBE contract", pixelOffset, c, decoded.Levels[0][pixelOffset+c], want)
			}
		}

		if decoded.Levels[0][pixelOffset+3] != 255 {
			t.Fatal("nonopaque output")
		}
	}
}

func TestLDRCoefficientBlackAndAdmission(t *testing.T) {
	t.Parallel()

	black := ldrTestTexture(4, 4, [4]byte{255, 128, 64, 0}, texture.Linear)
	for _, space := range []texture.ColorSpace{texture.Linear, texture.SRGB} {
		output, scale, err := PrepareLDRLightmapCoefficients([][]byte{black}, 7, space)
		if err != nil || scale != 7 {
			t.Fatal(scale, err)
		}

		decoded, decodeErr := ktx2data.Decode(output[0])
		if decodeErr != nil || !bytes.Equal(decoded.Levels[0][:4], []byte{0, 0, 0, 255}) {
			t.Fatal("black RGBE not retained", decoded, decodeErr)
		}
	}

	valid := ldrTestTexture(4, 4, [4]byte{255, 128, 64, 128}, texture.Linear)

	mip := encodeKTX(
		[]*image.NRGBA{
			{Pix: bytes.Repeat([]byte{255, 128, 64, 128}, 16), Stride: 16, Rect: image.Rect(0, 0, 4, 4)},
			{Pix: bytes.Repeat([]byte{255, 128, 64, 128}, 4), Stride: 8, Rect: image.Rect(0, 0, 2, 2)},
		},
		false,
	)
	for _, bad := range [][][]byte{nil, {valid, valid}, {valid, nil, valid}, {valid, valid, ldrTestTexture(8, 4, [4]byte{0, 0, 0, 0}, texture.Linear)}, {mip}, {ldrTestTexture(4, 4, [4]byte{255, 128, 64, 128}, texture.SRGB)}} {
		if _, _, err := PrepareLDRLightmapCoefficients(bad, 1, texture.Linear); err == nil {
			t.Fatal("malformed family accepted")
		}
	}

	for _, scale := range []float32{0, -1, float32(math.NaN()), float32(math.Inf(1))} {
		if _, _, err := PrepareLDRLightmapCoefficients([][]byte{valid}, scale, texture.Linear); err == nil {
			t.Fatal("invalid physical scale", scale)
		}
	}

	if _, _, err := PrepareLDRLightmapCoefficients([][]byte{valid}, 1, texture.ColorSpace(255)); err == nil {
		t.Fatal("invalid transfer accepted")
	}

	overflow := ldrTestTexture(4, 4, [4]byte{255, 0, 0, 255}, texture.Linear)
	if _, _, err := PrepareLDRLightmapCoefficients([][]byte{overflow}, 4, texture.Linear); err == nil {
		t.Fatal("physical scale overflow accepted")
	}
}

func ldrTestTexture(width, height int, pixel [4]byte, space texture.ColorSpace) []byte {
	return encodeKTX(
		[]*image.NRGBA{{Pix: bytes.Repeat(pixel[:], width*height), Stride: width * 4, Rect: image.Rect(0, 0, width, height)}},
		space == texture.SRGB,
	)
}
