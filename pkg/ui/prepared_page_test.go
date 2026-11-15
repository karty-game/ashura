package ui

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"testing"

	"github.com/karty-game/karty-webgpu/pkg/texture"
	"golang.org/x/image/font/gofont/goregular"
)

func TestCompressedImageAssetPreservesRegionsAndAccounting(t *testing.T) {
	t.Parallel()

	blocks := make([]byte, 16)
	blocks[0] = 7

	asset, err := NewPreparedImageAsset(8, 4, texture.BC1, blocks)
	if err != nil {
		t.Fatal(err)
	}

	blocks[0] = 0

	region, err := asset.Region(Rect{X: 2, Y: 1, W: 3, H: 2})
	if err != nil {
		t.Fatal(err)
	}

	node, err := NewImageFromAsset(region)
	if err != nil {
		t.Fatal(err)
	}

	if asset.page != region.page || asset.page.Format != texture.BC1 || asset.page.Pixels[0] != 7 ||
		preparedImageBytes(node) != 16 || region.uv != (Rect{X: 0.25, Y: 0.25, W: 0.375, H: 0.5}) {
		t.Fatal("compressed image lost owned blocks, region coordinates or exact accounting")
	}
}

func TestPreparedFontHashesCompressedPayload(t *testing.T) {
	t.Parallel()

	blocks := make([]byte, 16)
	metrics := make([]byte, 128)
	copy(metrics, "AFNT0001")
	binary.LittleEndian.PutUint32(metrics[8:], 4)
	binary.LittleEndian.PutUint32(metrics[12:], 4)
	binary.LittleEndian.PutUint32(metrics[16:], 32)
	binary.LittleEndian.PutUint32(metrics[20:], 6)
	binary.LittleEndian.PutUint32(metrics[24:], 1)

	fontHash, pixelHash := sha256.Sum256(goregular.TTF), sha256.Sum256(blocks)
	copy(metrics[28:60], fontHash[:])
	copy(metrics[60:92], pixelHash[:])

	face, err := LoadPreparedFont(goregular.TTF, metrics, blocks, texture.BC7)
	if err != nil {
		t.Fatal(err)
	}

	blocks[0] = 1

	if face.page.Format != texture.BC7 || len(face.page.Pixels) != 16 || face.page.Pixels[0] != 0 {
		t.Fatal("font failed to retain owned GPU-ready blocks")
	}

	if _, err := LoadPreparedFont(goregular.TTF, metrics, blocks, texture.BC7); !errors.Is(err, ErrBounds) {
		t.Fatal("font admitted a payload with the wrong checksum", err)
	}

	if _, err := LoadFont(goregular.TTF, metrics, face.page.Pixels); !errors.Is(err, ErrBounds) {
		t.Fatal("RGBA8 constructor admitted block bytes", err)
	}
}
