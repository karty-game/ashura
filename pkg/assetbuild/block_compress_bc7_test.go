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

func TestCompressBC7RetainsAlphaTransferAndAuthoredMips(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("CC", "must-not-run")

	ctx, closeCodec := buildCodecContext(t.Context())
	defer func() { _ = closeCodec() }()

	for _, space := range []texturedata.ColorSpace{texturedata.Linear, texturedata.SRGB} {
		levels := bc1CodecTestMips(false)
		for _, level := range levels {
			for offset := 3; offset < len(level); offset += 4 {
				level[offset] = 96
			}
		}

		source := bc1CodecTestSource(t, levels, space)
		before := bytes.Clone(source)

		encoded, err := CompressBC7(ctx, source, BlockCompressionOptions{})
		if err != nil {
			t.Fatal("BC7 encoding", space, err)
		}

		decoded, err := ktx2data.Decode(encoded)
		if err != nil || decoded.Format != texturedata.BC7 || decoded.ColorSpace != space ||
			decoded.Width != 12 || decoded.Height != 12 || len(decoded.Levels) != len(levels) {
			t.Fatal("BC7 authored dimensions, transfer or mips changed", decoded, err)
		}

		for level, blocks := range decoded.Levels {
			side := max(1, 12>>level)
			if len(blocks) != ((side+3)/4)*((side+3)/4)*16 {
				t.Fatal("BC7 block payload bound", level, len(blocks))
			}
		}

		if !bytes.Equal(source, before) || binary.LittleEndian.Uint64(encoded[104:]) >= binary.LittleEndian.Uint64(encoded[80:]) {
			t.Fatal("BC7 source mutated or mip payloads not smallest-first")
		}

		opaque, err := CompressBC7(ctx, bc1CodecTestSource(t, bc1CodecTestMips(false), space), BlockCompressionOptions{})
		if err != nil {
			t.Fatal(err)
		}

		opaqueData, err := ktx2data.Decode(opaque)
		if err != nil || bytes.Equal(decoded.Levels[0], opaqueData.Levels[0]) {
			t.Fatal("fractional alpha was discarded before BC7 encoding", err)
		}
	}
}

func TestCompressBC7RejectsInputAndPreservesCancellation(t *testing.T) {
	t.Parallel()

	for _, input := range [][]byte{nil, []byte("not KTX2"), ldrTestTexture(3, 4, [4]byte{1, 2, 3, 4}, texturedata.Linear)} {
		if _, err := CompressBC7(t.Context(), input, BlockCompressionOptions{}); !errors.Is(err, errBlockCompression) {
			t.Fatal("invalid BC7 source admitted", err)
		}
	}

	valid := ldrTestTexture(4, 4, [4]byte{1, 2, 3, 4}, texturedata.SRGB)
	for _, quality := range []int{-1, 5} {
		if _, err := CompressBC7(t.Context(), valid, BlockCompressionOptions{LDRQuality: &quality}); !errors.Is(err, errBlockCompression) {
			t.Fatal("invalid BC7 quality admitted", err)
		}
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := CompressBC7(ctx, valid, BlockCompressionOptions{}); !errors.Is(err, context.Canceled) {
		t.Fatal("BC7 lost cancellation", err)
	}
}
