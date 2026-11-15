//go:build !js

package assetbuild

import (
	"bytes"
	"encoding/binary"
	"image"
	"testing"

	"github.com/karty-game/karty-webgpu/internal/ktx2data"
	"github.com/karty-game/karty-webgpu/pkg/texture"
)

func TestNativeSSBumpRGBA8PreservesAuthoredChannelsAndMips(t *testing.T) {
	t.Parallel()

	for _, shape := range [][2]int{{8, 8}, {7, 5}, {1, 1}} {
		source := nativeSSBumpRGBATestSource(shape[0], shape[1])
		before := bytes.Clone(source)

		expected, err := ktx2data.Decode(source)
		if err != nil {
			t.Fatal(err)
		}

		output, err := PrepareNativeSSBumpRGBA8(source)
		if err != nil {
			t.Fatal(shape, err)
		}

		actual, err := ktx2data.Decode(output)
		if err != nil || actual.Width != expected.Width || actual.Height != expected.Height || actual.Format != texture.RGBA8 ||
			actual.ColorSpace != texture.Linear ||
			len(actual.Levels) != len(expected.Levels) {
			t.Fatal("raw SSBump metadata", shape, actual, err)
		}

		for level, pixels := range expected.Levels {
			if !bytes.Equal(pixels, actual.Levels[level]) {
				t.Fatal("authored RGB/alpha bytes changed", shape, level)
			}

			if level+1 < len(actual.Levels) &&
				binary.LittleEndian.Uint64(output[80+level*24:]) <= binary.LittleEndian.Uint64(output[80+(level+1)*24:]) {
				t.Fatal("RGBA mip payloads not canonical smallest-first")
			}
		}

		if !bytes.Equal(before, source) {
			t.Fatal("raw source mutated")
		}
	}
}

func TestNativeSSBumpRGBA8RejectsOtherRepresentations(t *testing.T) {
	t.Parallel()

	for _, source := range [][]byte{nil, ldrTestTexture(4, 4, [4]byte{1, 2, 3, 0}, texture.SRGB), writeNativeBlockKTX(blockVKBC7, 4, 4, [][]byte{make([]byte, 16)}), blockTestUASTC()} {
		if _, err := PrepareNativeSSBumpRGBA8(source); err == nil {
			t.Fatal("nonlinear/compressed source accepted")
		}
	}
}

func nativeSSBumpRGBATestSource(width, height int) []byte {
	var levels []*image.NRGBA

	for level := 0; ; level++ {
		pixels := make([]byte, width*height*imageChannels)
		for pixel := 0; pixel < len(pixels); pixel += imageChannels {
			index := pixel / imageChannels
			pixels[pixel], pixels[pixel+1], pixels[pixel+2], pixels[pixel+3] = byte(
				(index*17+level*31)&255,
			), byte(
				(index*29+level*11)&255,
			), byte(
				(index*13+level*7)&255,
			), []byte{0, 64, 128, 255}[index%4]
		}

		levels = append(levels, &image.NRGBA{Pix: pixels, Stride: width * imageChannels, Rect: image.Rect(0, 0, width, height)})
		if width == 1 && height == 1 {
			break
		}

		width, height = max(1, width>>1), max(1, height>>1)
	}

	canonical := encodeKTX(levels, false)
	info, _ := ktx2data.Decode(canonical)
	smallest := binary.LittleEndian.Uint64(canonical[80+(len(levels)-1)*24:])
	// Deliberately exercise an admitted older largest-first physical payload order.
	source := bytes.Clone(canonical[:smallest])
	for level, pixels := range info.Levels {
		binary.LittleEndian.PutUint64(source[80+level*24:], uint64(len(source)))
		source = append(source, pixels...)
	}

	return source
}
