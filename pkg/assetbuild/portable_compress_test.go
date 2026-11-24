//go:build !js

package assetbuild

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"math"
	"testing"

	"github.com/karty-game/ashura/internal/ktx2data"
	"github.com/karty-game/ashura/internal/texturedata"
	"github.com/karty-game/ashura/pkg/texture"
)

func TestPortableRGBESharedExponentsPreservePhysicalRange(t *testing.T) {
	t.Parallel()

	pixels := make([]byte, 8*8*4)
	// Adjacent blocks preserve radically different physical exposures. Mantissa
	// rescaling happens only within each block; page scale remains unchanged.
	for pixelY := range 8 {
		for pixelX := range 8 {
			offset := (pixelY*8 + pixelX) * 4

			exponent := byte(130)
			if pixelX >= 4 {
				exponent = 100
			}

			copy(pixels[offset:], []byte{241, 129, 17, exponent - byte(pixelY%4)})
		}
	}

	pixels[3] = 0 // RGB associated with an unused exponent is black.
	input := texturedata.Data{Width: 8, Height: 8, ColorSpace: texturedata.Linear, Levels: [][]byte{pixels}}
	before := bytes.Clone(pixels)

	mantissas, exponents := portableRGBEBlocks(input)
	if !bytes.Equal(pixels, before) || len(exponents) != 4 || exponents[0] != 130 || exponents[1] != 100 {
		t.Fatal("block exponent selection or source immutability lost", exponents)
	}

	for pixelY := range 8 {
		for pixelX := range 8 {
			offset := (pixelY*8 + pixelX) * 4
			block := (pixelY/4)*2 + pixelX/4

			for channel := range 3 {
				original := decodeBlockRGBE(pixels[offset:], channel)
				decoded := float32(math.Ldexp(float64(mantissas[offset+channel])/255, int(exponents[block])-128))

				bound := float32(math.Ldexp(.5/255, int(exponents[block])-128))
				if math.Abs(float64(original-decoded)) > float64(bound)*1.0001 {
					t.Fatalf("physical mantissa error exceeded nearest-byte bound: %g %g", original, decoded)
				}
			}

			if mantissas[offset+3] != 255 {
				t.Fatal("color encoder must receive opaque alpha")
			}
		}
	}
}

func TestPortableNativeKTXDescriptorsAndWholeMipBlocks(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		format   uint32
		bytes    int
		encoding texturedata.Format
		space    texturedata.ColorSpace
	}{
		{blockVKBC1SRGB, 8, texturedata.BC1, texturedata.SRGB},
		{blockVKETC2RGB, 8, texturedata.ETC2RGB, texturedata.Linear},
		{blockVKETC2RGBSRGB, 8, texturedata.ETC2RGB, texturedata.SRGB},
		{blockVKETC2RGBA, 16, texturedata.ETC2RGBA, texturedata.Linear},
		{blockVKASTC4x4, 16, texturedata.ASTC4x4, texturedata.Linear},
	} {
		levels := [][]byte{
			bytes.Repeat([]byte{1}, 4*test.bytes),
			bytes.Repeat([]byte{2}, test.bytes),
			bytes.Repeat([]byte{3}, test.bytes),
			bytes.Repeat([]byte{4}, test.bytes),
		}
		encoded := writeNativeBlockKTX(test.format, 8, 8, levels)

		image, err := ktx2data.Decode(encoded)
		if err != nil || image.Format != test.encoding || image.ColorSpace != test.space {
			t.Fatal("native descriptor admission", test.format, image, err)
		}

		for index := range levels {
			if !bytes.Equal(image.Levels[index], levels[index]) {
				t.Fatal("native block mip tail lost")
			}
		}
		//nolint:gosec // The fixed fixture table contains only 8- or 16-byte blocks.
		if binary.LittleEndian.Uint64(encoded[80:])%uint64(test.bytes) != 0 {
			t.Fatal("base mip is not block-byte aligned")
		}
	}
}

func TestPortableCodecsRejectInvalidInputsBeforeTools(t *testing.T) {
	t.Parallel()

	valid := blockTestRGBE(make([]byte, 8*8*4))
	if _, err := TranscodeAlbedoNative(t.Context(), valid, texture.BC1, BlockCompressionOptions{}); err == nil {
		t.Fatal("numeric texture accepted as opaque ETC1S albedo")
	}

	canceled, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := CompressRGBEETC2(
		canceled,
		valid,
		BlockCompressionOptions{},
	); !errors.Is(
		err,
		context.Canceled,
	) {
		t.Fatal("cancellation ignored", err)
	}

	if _, err := CompressSSBumpASTC(t.Context(), nil, BlockCompressionOptions{}); err == nil {
		t.Fatal("invalid source accepted")
	}
}

func TestPortableRGBEAlphaBlocksKeepEveryExponentExact(t *testing.T) {
	t.Parallel()

	colors := bytes.Repeat([]byte{1, 2, 3, 4, 5, 6, 7, 8}, 256)

	exponents := make([]byte, 256)
	for index := range exponents {
		exponents[index] = byte(index)
	}

	blocks := portableRGBAColorBlocks(colors, exponents)
	for index, exponent := range exponents {
		block := blocks[index*16 : (index+1)*16]
		if block[0] != exponent || !bytes.Equal(block[1:8], make([]byte, 7)) || !bytes.Equal(block[8:], colors[index*8:index*8+8]) {
			t.Fatal("EAC alpha or ETC color bits changed", index)
		}
	}
}
