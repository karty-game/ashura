//go:build !js

package assetbuild

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"testing"

	"github.com/karty-game/karty-webgpu/internal/ktx2data"
	"github.com/karty-game/karty-webgpu/pkg/texture"
)

func TestLDRNativeFormatsRetainTransferAndBlockBounds(t *testing.T) {
	t.Parallel()

	for _, space := range []texture.ColorSpace{texture.Linear, texture.SRGB} {
		for _, target := range []texture.Format{texture.BC1, texture.BC7, texture.ETC2RGB, texture.ETC2RGBA} {
			format, err := ldrNativeTarget(target, space)
			if err != nil {
				t.Fatal(err)
			}

			blockBytes := 8
			if target == texture.BC7 || target == texture.ETC2RGBA {
				blockBytes = 16
			}

			levels := [][]byte{bytes.Repeat([]byte{1}, 4*blockBytes), bytes.Repeat([]byte{2}, blockBytes)}
			encoded := writeCanonicalNativeBlockKTX(format, 8, 8, levels)

			decoded, err := ktx2data.Decode(encoded)
			if err != nil || decoded.Format != target || decoded.ColorSpace != space || len(decoded.Levels) != 2 {
				t.Fatal("native LDR metadata", format, decoded, err)
			}

			if binary.LittleEndian.Uint64(encoded[104:]) >= binary.LittleEndian.Uint64(encoded[80:]) {
				t.Fatal("native LDR mip payloads must be smallest first")
			}

			admitted, err := ktx2data.Decode(encoded)
			if err != nil || !bytes.Equal(admitted.Levels[0], levels[0]) || !bytes.Equal(admitted.Levels[1], levels[1]) {
				t.Fatal("native LDR levels changed", format, err)
			}
		}
	}

	if format, err := ldrNativeTarget(texture.BC7, texture.SRGB); err != nil || format != 146 {
		t.Fatal("BC7 sRGB Vulkan format", format, err)
	}
}

func TestLDRUASTCColorAdmissionPreservesPreparedData(t *testing.T) {
	t.Parallel()

	for _, space := range []texture.ColorSpace{texture.Linear, texture.SRGB} {
		prepared := ldrTestUASTC(space)
		before := bytes.Clone(prepared)

		info, err := inspectBlockUASTC(prepared)
		if err != nil || info.width != 8 || info.height != 8 || info.levels != 4 || info.space != space {
			t.Fatal(info, err)
		}

		rgba := bytes.Clone(prepared)

		rgba[binary.LittleEndian.Uint32(rgba[48:])+31] = 3
		if rgbaInfo, rgbaErr := inspectBlockUASTC(rgba); rgbaErr != nil || rgbaInfo.space != space {
			t.Fatal("numeric RGBA UASTC rejected", rgbaInfo, rgbaErr)
		}

		if !bytes.Equal(prepared, before) {
			t.Fatal("inspection altered intermediate")
		}
	}

	bad := ldrTestUASTC(texture.Linear)

	bad[binary.LittleEndian.Uint32(bad[48:])+14] = 3
	if _, err := inspectBlockUASTC(bad); err == nil {
		t.Fatal("unsupported transfer accepted")
	}
}

func TestLDRStagesRejectBeforeEncodingAndPreserveCancellation(t *testing.T) {
	t.Parallel()

	options := BlockCompressionOptions{}

	transparent := ldrTestTexture(4, 4, [4]byte{1, 2, 3, 254}, texture.Linear)
	if _, err := PrepareOpaqueLDRUASTC(t.Context(), transparent, options); !errors.Is(err, errBlockCompression) {
		t.Fatal("transparency reached encoder", err)
	}

	if _, err := CompressETC2RGB(t.Context(), transparent, options); !errors.Is(err, errBlockCompression) {
		t.Fatal("opaque ETC2 compression dropped source alpha", err)
	}

	for _, shape := range [][2]int{{1, 1}, {7, 5}} {
		if _, err := PrepareOpaqueLDRUASTC(
			t.Context(),
			ldrTestTexture(shape[0], shape[1], [4]byte{1, 2, 3, 255}, texture.Linear),
			options,
		); !errors.Is(
			err,
			errBlockCompression,
		) {
			t.Fatal("nonaligned native base reached codec", shape, err)
		}
	}

	for _, quality := range []int{-1, 5} {
		invalid := options

		invalid.LDRQuality = &quality
		if _, err := PrepareOpaqueLDRUASTC(
			t.Context(),
			ldrTestTexture(4, 4, [4]byte{1, 2, 3, 255}, texture.SRGB),
			invalid,
		); !errors.Is(
			err,
			errBlockCompression,
		) {
			t.Fatal("quality reached encoder", err)
		}
	}

	for _, target := range []texture.Format{texture.RGBA8, texture.BC6H, texture.ASTC4x4} {
		if _, err := TranscodeLDRNative(t.Context(), ldrTestUASTC(texture.SRGB), target, options); !errors.Is(err, errBlockCompression) {
			t.Fatal("unsupported LDR target reached codec", target, err)
		}
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := CompressETC2RGB(ctx, transparent, options); !errors.Is(err, context.Canceled) {
		t.Fatal("opaque ETC2 compression lost cancellation", err)
	}

	for _, space := range []texture.ColorSpace{texture.Linear, texture.SRGB} {
		if _, err := PrepareOpaqueLDRUASTC(
			ctx,
			ldrTestTexture(4, 4, [4]byte{1, 2, 3, 255}, space),
			options,
		); !errors.Is(
			err,
			context.Canceled,
		) {
			t.Fatal("LDR prepare cancellation", space, err)
		}

		for _, target := range []texture.Format{texture.BC1, texture.BC7, texture.ETC2RGB} {
			if _, err := TranscodeLDRNative(ctx, ldrTestUASTC(space), target, options); !errors.Is(err, context.Canceled) {
				t.Fatal("LDR transcode cancellation", space, target, err)
			}
		}
	}

	if _, err := TranscodeSSBumpNative(ctx, ldrTestUASTC(texture.Linear), texture.ETC2RGB, options); !errors.Is(err, context.Canceled) {
		t.Fatal("SSBump ETC2 target not admitted", err)
	}
}

func ldrTestUASTC(space texture.ColorSpace) []byte {
	prepared := blockTestUASTC()
	if space == texture.SRGB {
		prepared[binary.LittleEndian.Uint32(prepared[48:])+14] = 2
	}

	return prepared
}
