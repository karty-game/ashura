//go:build !js

package assetbuild

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"testing"
)

func TestBC1PatchPreservesExactMaskAndOpaqueBlocks(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		first, second uint16
		selectors     uint32
		alpha         byte
		preserve      bool
	}{
		{0xf800, 0x07e0, 0xffffffff, 255, true},  // Four-color selector3 is opaque.
		{0x07e0, 0xf800, 0xaaaaaaaa, 255, true},  // Safe three-color block.
		{0x07e0, 0xf800, 0xffffffff, 255, false}, // RGB black selector3 must become opaque.
		{0xf800, 0x07e0, 0, 0, false},            // Fully transparent clears all RGB.
	} {
		block := bc1TestBlock(test.first, test.second, test.selectors)
		before := bytes.Clone(block)

		var source [16][4]byte
		for index := range source {
			source[index] = [4]byte{0, 255, 0, test.alpha}
		}

		patchBC1Block(block, source)

		if test.preserve && !bytes.Equal(block, before) {
			t.Fatal("safe opaque block changed")
		}

		for pixel := range 16 {
			if got := bc1TestAlpha(block, pixel); got != test.alpha {
				t.Fatal("opaque/transparent block mask changed", test, pixel, got)
			}
		}
	}

	block := bc1TestBlock(0xf800, 0x07e0, 0)

	var source [16][4]byte
	for index := range source {
		source[index] = [4]byte{255, 0, 0, []byte{0, 127, 128, 255}[index%4]}
	}

	patchBC1Block(block, source)

	if binary.LittleEndian.Uint16(block) > binary.LittleEndian.Uint16(block[2:]) {
		t.Fatal("masked block did not select BC1 punchthrough mode")
	}

	for pixel := range source {
		want := byte(0)
		if source[pixel][3] >= 128 {
			want = 255
		}

		if got := bc1TestAlpha(block, pixel); got != want {
			t.Fatal("source threshold not preserved", pixel, got, want)
		}

		if want == 255 && binary.LittleEndian.Uint32(block[4:])>>(2*pixel)&3 != 1 {
			t.Fatal("foreground red did not select swapped red endpoint")
		}
	}
}

func TestBC1PatchHandlesSubBlockMipRowsAndCancellation(t *testing.T) {
	t.Parallel()

	for _, dimensions := range [][2]int{{12, 12}, {6, 6}, {3, 3}, {1, 1}, {12, 4}, {6, 2}} {
		width, height := dimensions[0], dimensions[1]

		pixels := make([]byte, width*height*4)
		for index := range width * height {
			copy(pixels[index*4:], []byte{255, 0, 0, byte(index%2) * 255})
		}

		blocks := bytes.Repeat(bc1TestBlock(0xf800, 0x07e0, 0), ((width+3)/4)*((height+3)/4))
		if err := patchBC1Level(t.Context(), width, height, pixels, blocks); err != nil {
			t.Fatal(dimensions, err)
		}

		for y := range height {
			for x := range width {
				block := (y/4*((width+3)/4) + x/4) * 8
				if got := bc1TestAlpha(blocks[block:], y%4*4+x%4); got != pixels[(y*width+x)*4+3] {
					t.Fatal("physical block rows changed mip coverage", dimensions, x, y)
				}
			}
		}
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if err := patchBC1Level(ctx, 4, 4, make([]byte, 64), make([]byte, 8)); !errors.Is(err, context.Canceled) {
		t.Fatal("mask patch lost cancellation", err)
	}

	if err := patchBC1Level(t.Context(), 4, 4, make([]byte, 64), make([]byte, 7)); !errors.Is(err, errBlockCompression) {
		t.Fatal("short block payload admitted", err)
	}
}

func bc1TestBlock(first, second uint16, selectors uint32) []byte {
	block := make([]byte, 8)
	binary.LittleEndian.PutUint16(block, first)
	binary.LittleEndian.PutUint16(block[2:], second)
	binary.LittleEndian.PutUint32(block[4:], selectors)

	return block
}

func bc1TestAlpha(block []byte, pixel int) byte {
	if binary.LittleEndian.Uint16(block) <= binary.LittleEndian.Uint16(block[2:]) &&
		binary.LittleEndian.Uint32(block[4:])>>(2*pixel)&3 == 3 {
		return 0
	}

	return 255
}
