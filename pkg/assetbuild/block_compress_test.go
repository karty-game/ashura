//go:build !js

package assetbuild

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"image"
	"math"
	"testing"
	"time"

	"github.com/karty-game/ashura/internal/ktx2data"
	"github.com/karty-game/ashura/internal/texturedata"
)

func TestNormalizeRGBERNMPreservesPhysicalRadiance(t *testing.T) {
	t.Parallel()

	var source [3][]byte
	for coefficient := range source {
		pixels := make([]byte, 8*8*4)
		copy(pixels, []byte{255, 127, 1, 128 - byte(coefficient)*12})
		copy(pixels[4:], []byte{255, 1, 1, 0}) // Unused RGB with zero exponent is black.
		source[coefficient] = blockTestRGBE(pixels)
	}

	before := [3][]byte{}
	for index := range source {
		before[index] = bytes.Clone(source[index])
	}

	normalized, scale, err := NormalizeRGBERNM(source, 3.5)
	if err != nil {
		t.Fatal(err)
	}

	if scale != 3.5/4096 {
		t.Fatalf("incorrect exposure compensation: %g", scale)
	}

	for index := range source {
		if !bytes.Equal(source[index], before[index]) {
			t.Fatal("normalization mutated source")
		}

		oldImage, oldErr := ktx2data.Decode(source[index])

		newImage, newErr := ktx2data.Decode(normalized[index])
		if oldErr != nil || newErr != nil {
			t.Fatal(oldErr, newErr)
		}

		for pixel := 0; pixel < len(oldImage.Levels[0]); pixel += 4 {
			for channel := range 3 {
				oldValue := decodeBlockRGBE(oldImage.Levels[0][pixel:], channel) * 3.5

				newValue := decodeBlockRGBE(newImage.Levels[0][pixel:], channel) * scale
				if oldValue != newValue {
					t.Fatalf("physical radiance changed: %g != %g", oldValue, newValue)
				}
			}
		}
	}
}

func TestNormalizeRGBERNMRejectsMalformedPage(t *testing.T) {
	t.Parallel()

	valid := blockTestRGBE(make([]byte, 8*8*4))
	for _, scale := range []float32{0, -1, float32(math.Inf(1)), float32(math.NaN())} {
		if _, _, err := NormalizeRGBERNM([3][]byte{valid, valid, valid}, scale); err == nil {
			t.Fatal("invalid scale accepted")
		}
	}

	if _, _, err := NormalizeRGBERNM([3][]byte{valid, nil, valid}, 1); err == nil {
		t.Fatal("missing coefficient accepted")
	}

	output, scale, err := NormalizeRGBERNM([3][]byte{valid, valid, valid}, 2)
	if err != nil || scale != 2 || !bytes.Equal(output[0], valid) {
		t.Fatal("black page changed", scale, err)
	}
}

func TestRGBEFloatPixelsUseKartyNumerics(t *testing.T) {
	t.Parallel()

	pixels := make([]byte, 8*8*4)
	copy(pixels, []byte{255, 128, 1, 128})
	copy(pixels[4:], []byte{255, 255, 255, 0})
	value := texturedata.Data{Width: 8, Height: 8, Levels: [][]byte{pixels}}

	data, err := rgbeFloatPixels(value)
	if err != nil {
		t.Fatal(err)
	}

	for channel, expected := range []float32{1, 128.0 / 255, 1.0 / 255, 1, 0, 0, 0, 1} {
		actual := math.Float32frombits(binary.LittleEndian.Uint32(data[channel*4:]))
		if actual != expected {
			t.Fatalf("channel %d: %g != %g", channel, actual, expected)
		}
	}

	pixels[3] = 255

	if _, err := rgbeFloatPixels(value); err == nil {
		t.Fatal("half-float overflow accepted")
	}
}

func TestRGBEFloatPixelsRejectOversizedOrIncompleteRequests(t *testing.T) {
	t.Parallel()

	for _, image := range []texturedata.Data{
		{Width: 16384, Height: 16384},
		{Width: 8, Height: 8},
		{Width: 8, Height: 8, Levels: [][]byte{make([]byte, 255)}},
	} {
		if _, err := rgbeFloatPixels(image); !errors.Is(err, errBlockCompression) {
			t.Fatal("invalid float request reached allocation", err)
		}
	}
}

func TestNormalizeRGBERNMRejectsUnrepresentableExponentSpan(t *testing.T) {
	t.Parallel()

	pixels := make([]byte, 8*8*4)
	copy(pixels, []byte{255, 255, 255, 1, 255, 255, 255, 255})

	source := blockTestRGBE(pixels)
	if _, _, err := NormalizeRGBERNM([3][]byte{source, source, source}, 1); err == nil {
		t.Fatal("exponent span that would turn a lit texel black was accepted")
	}

	pixels[3] = 2
	source = blockTestRGBE(pixels)

	output, scale, err := NormalizeRGBERNM([3][]byte{source, source, source}, 1)
	if err != nil {
		t.Fatal(err)
	}

	decoded, err := ktx2data.Decode(output[0])
	if err != nil {
		t.Fatal(err)
	}

	if decoded.Levels[0][3] != 1 || decoded.Levels[0][7] != 254 || scale != 2 {
		t.Fatal("representable boundary span was not preserved")
	}
}

func TestNativeBlockContainersPreserveMipsAndRejectOverlap(t *testing.T) {
	t.Parallel()

	for _, format := range []uint32{blockVKBC6H, blockVKBC7} {
		levels := [][]byte{bytes.Repeat([]byte{1}, 64), bytes.Repeat([]byte{2}, 16), bytes.Repeat([]byte{3}, 16)}
		encoded := writeNativeBlockKTX(format, 8, 8, levels)

		decoded, err := ktx2data.Decode(encoded)
		if err != nil {
			t.Fatal(err)
		}

		for index := range levels {
			if !bytes.Equal(decoded.Levels[index], levels[index]) {
				t.Fatal("mip changed")
			}
		}

		binary.LittleEndian.PutUint64(encoded[104:], binary.LittleEndian.Uint64(encoded[80:]))

		if _, err = ktx2data.Decode(encoded); err == nil {
			t.Fatal("overlapping mip accepted")
		}
	}
}

func TestBlockCompressionRejectsBeforeEncoding(t *testing.T) {
	t.Parallel()

	if _, err := CompressRGBERNM(t.Context(), nil, BlockCompressionOptions{}); err == nil {
		t.Fatal("invalid source accepted")
	}

	if _, err := CompressSSBumpBC7(t.Context(), nil, BlockCompressionOptions{}); err == nil {
		t.Fatal("invalid source accepted")
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, _, err := blockContext(ctx, time.Second); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	//nolint:staticcheck // SA1012: nil intentionally verifies rejection of an invalid context.
	if _, _, err := blockContext(nil, 0); err == nil {
		t.Fatal("nil context accepted")
	}
}

func blockTestRGBE(pixels []byte) []byte {
	return encodeKTX([]*image.NRGBA{{Pix: pixels, Stride: 8 * 4, Rect: image.Rect(0, 0, 8, 8)}}, false)
}
