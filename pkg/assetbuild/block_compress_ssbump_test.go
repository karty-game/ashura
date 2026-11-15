//go:build !js

package assetbuild

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"testing"

	"github.com/karty-game/karty-webgpu/pkg/texture"
)

func TestSSBumpUASTCAdmissionPreservesMipsAndSource(t *testing.T) {
	t.Parallel()

	prepared := blockTestUASTC()
	before := bytes.Clone(prepared)

	width, height, count, err := inspectSSBumpUASTC(prepared)
	if err != nil || width != 8 || height != 8 || count != 4 {
		t.Fatal("authored coefficient mip chain rejected", width, height, count, err)
	}

	if !bytes.Equal(before, prepared) {
		t.Fatal("admission mutated the UASTC artifact")
	}
}

func TestSSBumpUASTCRejectsInvalidIntermediate(t *testing.T) {
	t.Parallel()

	cases := map[string]func([]byte){
		"native-format":       func(data []byte) { binary.LittleEndian.PutUint32(data[12:], blockVKASTC4x4) },
		"wrong-model":         func(data []byte) { data[binary.LittleEndian.Uint32(data[48:])+12] = 162 },
		"srgb":                func(data []byte) { data[binary.LittleEndian.Uint32(data[48:])+14] = 2 },
		"supercompressed":     func(data []byte) { binary.LittleEndian.PutUint32(data[44:], 2) },
		"overlapping-mips":    func(data []byte) { copy(data[104:112], data[80:88]) },
		"truncated-mip":       func(data []byte) { binary.LittleEndian.PutUint64(data[88:], 1) },
		"descriptor-overflow": func(data []byte) { binary.LittleEndian.PutUint32(data[48:], ^uint32(0)) },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			prepared := blockTestUASTC()
			mutate(prepared)

			if _, _, _, err := inspectSSBumpUASTC(prepared); err == nil {
				t.Fatal("malformed UASTC artifact accepted")
			}
		})
	}

	if _, _, _, err := inspectSSBumpUASTC(make([]byte, 79)); err == nil {
		t.Fatal("truncated header accepted")
	}
}

func TestSSBumpStagesRejectBeforeEncoding(t *testing.T) {
	t.Parallel()

	options := BlockCompressionOptions{}

	prepared := blockTestUASTC()
	if _, err := TranscodeSSBumpNative(t.Context(), prepared, texture.RGBA8, options); err == nil {
		t.Fatal("unsupported target accepted")
	}

	if _, err := PrepareSSBumpUASTC(t.Context(), prepared, options); err == nil {
		t.Fatal("UASTC accepted as uncompressed source")
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := TranscodeSSBumpNative(ctx, prepared, texture.BC7, options); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation did not stop before encoding", err)
	}

	if _, err := PrepareSSBumpUASTC(ctx, blockTestRGBE(make([]byte, 8*8*4)), options); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation did not stop before encoding", err)
	}
}

func blockTestUASTC() []byte {
	data := writeNativeBlockKTX(blockVKASTC4x4, 8, 8, [][]byte{
		bytes.Repeat([]byte{1}, 64), bytes.Repeat([]byte{2}, 16), bytes.Repeat([]byte{3}, 16), bytes.Repeat([]byte{4}, 16),
	})
	binary.LittleEndian.PutUint32(data[12:], 0)
	data[binary.LittleEndian.Uint32(data[48:])+12] = 166

	return data
}

func TestOfflineUASTCQualityPreservesDefaultAndAdmitsExplicitZero(t *testing.T) {
	t.Parallel()

	for _, fallback := range []int{portableRNMQuality, portableSSBumpQuality} {
		quality, err := blockUASTCQuality(nil, fallback)
		if err != nil || quality != fallback {
			t.Fatal("omitted quality changed the existing profile", quality, fallback, err)
		}
	}

	for _, selected := range []int{0, 1, 2, 3, 4} {
		quality, err := blockUASTCQuality(&selected, portableSSBumpQuality)
		if err != nil || quality != selected {
			t.Fatal("explicit offline effort was not retained", quality, selected, err)
		}
	}

	for _, selected := range []int{-1, maxBlockUASTCQuality + 1} {
		if _, err := blockUASTCQuality(&selected, portableSSBumpQuality); err == nil {
			t.Fatal("out-of-profile quality accepted")
		}

		options := BlockCompressionOptions{SSBumpQuality: &selected, RGBEQuality: &selected}
		if _, err := PrepareSSBumpUASTC(t.Context(), blockTestRGBE(make([]byte, 8*8*4)), options); err == nil {
			t.Fatal("invalid coefficient quality reached encoder startup")
		}

		if _, err := CompressRGBEETC2(t.Context(), blockTestRGBE(make([]byte, 8*8*4)), options); err == nil {
			t.Fatal("invalid mobile lightmap quality reached encoder startup")
		}
	}
}
