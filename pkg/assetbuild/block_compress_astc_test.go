//go:build !js

package assetbuild

import (
	"encoding/binary"
	"testing"

	"github.com/karty-game/ashura/internal/ktx2data"
	"github.com/karty-game/ashura/internal/texturedata"
)

func TestTiledASTCPreservesBlockRowsAndIndependentAuthoredMips(t *testing.T) {
	t.Parallel()

	const width, height = 18, 312

	var (
		levels [][]byte
		colors [][4]byte
	)

	for level, mipWidth, mipHeight := 0, width, height; ; level, mipWidth, mipHeight = level+1, max(1, mipWidth>>1), max(1, mipHeight>>1) {
		color := [4]byte{byte(30 + level*17), byte(70 + level*7), byte(180 - level*11), byte(99 + level*9)}
		colors = append(colors, color)

		pixels := make([]byte, mipWidth*mipHeight*4)
		for offset := 0; offset < len(pixels); offset += 4 {
			copy(pixels[offset:], color[:])

			if level == 0 && offset >= width*astcTileRows*4 {
				copy(pixels[offset:], []byte{219, 31, 27, 181})
			}
		}

		levels = append(levels, pixels)

		if mipWidth == 1 && mipHeight == 1 {
			break
		}
	}

	source, err := EncodeTexture(Image{Name: "Tiles", Source: "tiles", Usage: UsageNormal, Width: width, Height: height,
		Levels: levels, MaxMipLevels: len(levels), Atlas: true})
	if err != nil {
		t.Fatal(err)
	}

	encoded, err := CompressASTC6x6(t.Context(), source, BlockCompressionOptions{})
	if err != nil {
		t.Fatal(err)
	}

	image, err := ktx2data.Decode(encoded)
	if err != nil || image.Format != texturedata.ASTC6x6 || image.ColorSpace != texturedata.Linear || len(image.Levels) != len(levels) {
		t.Fatal("tiled native mip metadata", image, err)
	}

	checkASTCConstantMips(t, image.Levels, colors, width)
}

func checkASTCConstantMips(t *testing.T, levels [][]byte, colors [][4]byte, width int) {
	t.Helper()

	for level, blocks := range levels {
		for offset := 0; offset < len(blocks); offset += 16 {
			want := colors[level]
			if level == 0 && offset >= width/astcSide*(astcTileRows/astcSide)*16 {
				want = [4]byte{219, 31, 27, 181}
			}
			// Constant ASTC blocks use the independently specified UNORM16
			// void-extent form. All four stored channels must remain independent.
			if binary.LittleEndian.Uint16(blocks[offset:])&0x1ff != 0x1fc {
				t.Fatal("constant block is not void extent", level, offset)
			}

			for channel, value := range want {
				if actual := binary.LittleEndian.Uint16(blocks[offset+8+channel*2:]); actual != uint16(value)*257 {
					t.Fatal("tile/mip/channel changed", level, offset, channel, actual, value)
				}
			}
		}
	}
}
