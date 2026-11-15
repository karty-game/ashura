//go:build !js

package assetbuild

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"testing"

	"github.com/karty-game/karty-webgpu/internal/ktx2data"
	"github.com/karty-game/karty-webgpu/internal/texturedata"
)

func TestCompressBC1PreservesAuthoredCoverageWithoutExecutables(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("CC", "must-not-run")
	t.Setenv("CXX", "must-not-run")

	ctx, closeCodec := buildCodecContext(t.Context())
	defer func() { _ = closeCodec() }()

	for _, space := range []texturedata.ColorSpace{texturedata.Linear, texturedata.SRGB} {
		for _, masked := range []bool{true, false} {
			levels := bc1CodecTestMips(masked)
			source := bc1CodecTestSource(t, levels, space)
			before := bytes.Clone(source)

			encoded, err := CompressBC1(ctx, source, BlockCompressionOptions{})
			if err != nil {
				t.Fatal("portable BC1 encoder", space, masked, err)
			}

			decoded, err := ktx2data.Decode(encoded)
			if err != nil || decoded.Format != texturedata.BC1 || decoded.ColorSpace != space ||
				decoded.Width != 12 || decoded.Height != 12 || len(decoded.Levels) != len(levels) || !ktx2data.BC1Alpha(encoded) {
				t.Fatal("compressed profile/mip admission", decoded, err)
			}

			assertBC1CodecCoverage(t, decoded, levels)

			if !masked && space == texturedata.Linear {
				assertBC1NumericDepth(t, decoded.Levels[0])
			}

			if !bytes.Equal(source, before) {
				t.Fatal("BC1 preparation mutated source KTX2")
			}

			if binary.LittleEndian.Uint64(encoded[104:]) >= binary.LittleEndian.Uint64(encoded[80:]) {
				t.Fatal("BC1 authored mip payloads are not stored smallest-first")
			}
		}
	}
}

func TestCompressBC1RejectsUnsupportedInputAndCancellation(t *testing.T) {
	t.Parallel()

	valid := bc1CodecTestSource(t, bc1CodecTestMips(true), texturedata.Linear)
	for _, input := range [][]byte{
		nil, []byte("not KTX2"), ldrTestTexture(3, 4, [4]byte{255, 255, 255, 255}, texturedata.Linear),
		writeCanonicalNativeBlockKTX(blockVKBC1RGBA, 4, 4, [][]byte{make([]byte, 8)}),
	} {
		if _, err := CompressBC1(t.Context(), input, BlockCompressionOptions{}); !errors.Is(err, errBlockCompression) {
			t.Fatal("invalid BC1 input reached codec", err)
		}
	}

	for _, quality := range []int{-1, 5} {
		if _, err := CompressBC1(t.Context(), valid, BlockCompressionOptions{LDRQuality: &quality}); !errors.Is(err, errBlockCompression) {
			t.Fatal("invalid BC1 quality reached codec", err)
		}
	}
	//nolint:staticcheck // Deliberately verify rejection of the public API's invalid nil context.
	if _, err := CompressBC1(nil, valid, BlockCompressionOptions{}); !errors.Is(err, errBlockCompression) {
		t.Fatal("nil context admitted", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := CompressBC1(ctx, valid, BlockCompressionOptions{}); !errors.Is(err, context.Canceled) {
		t.Fatal("BC1 compression lost cancellation", err)
	}
}

func bc1CodecTestMips(masked bool) [][]byte {
	levels := make([][]byte, 0, 4)
	for _, size := range []int{12, 6, 3, 1} {
		pixels := make([]byte, size*size*4)
		for index := range size * size {
			alpha := byte(255)
			if masked {
				alpha = []byte{0, 127, 128, 255}[index%4]
			}
			//nolint:gosec // The column ramp is explicitly bounded to 0..255.
			copy(pixels[index*4:], []byte{128, byte(index % size * 255 / max(1, size-1)), 128, alpha})
		}

		levels = append(levels, pixels)
	}

	return levels
}

func bc1CodecTestSource(t *testing.T, levels [][]byte, space texturedata.ColorSpace) []byte {
	t.Helper()

	usage := UsageLightmap
	if space == texturedata.SRGB {
		usage = UsageAlbedo
	}

	encoded, err := EncodeTexture(Image{Name: "bc1", Source: "bc1.png", Usage: usage, Width: 12, Height: 12, Levels: levels})
	if err != nil {
		t.Fatal(err)
	}

	return encoded
}

func assertBC1CodecCoverage(t *testing.T, image texturedata.Data, source [][]byte) {
	t.Helper()

	for level, blocks := range image.Levels {
		size := max(1, image.Width>>level)
		for row := range size {
			for column := range size {
				block := (row/4*((size+3)/4) + column/4) * 8

				want := byte(0)
				if source[level][(row*size+column)*4+3] >= 128 {
					want = 255
				}

				if got := bc1TestAlpha(blocks[block:], row%4*4+column%4); got != want {
					t.Fatal("authored source mask lost", level, column, row, got, want)
				}
			}
		}
	}
}

func assertBC1NumericDepth(t *testing.T, blocks []byte) {
	t.Helper()

	values := make(map[int]bool)

	for column := range 12 {
		block := blocks[(column/4)*8:]
		first, second := binary.LittleEndian.Uint16(block), binary.LittleEndian.Uint16(block[2:])
		green0, green1 := int(first>>5&63), int(second>>5&63)

		palette := [4]int{green0, green1, (2*green0 + green1) / 3, (green0 + 2*green1) / 3}
		if first <= second {
			palette[2], palette[3] = (green0+green1)/2, 0
		}

		selector := binary.LittleEndian.Uint32(block[4:]) >> (2 * (column % 4)) & 3
		values[palette[selector]] = true
	}

	if len(values) < 3 {
		t.Fatal("continuous numeric depth collapsed to binary coverage", values)
	}
}
