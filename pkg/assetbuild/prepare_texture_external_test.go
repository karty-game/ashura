//go:build !js

package assetbuild_test

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
	"github.com/karty-game/karty-webgpu/pkg/texture"
	"github.com/karty-game/karty-webgpu/pkg/texture/ktx2"
)

func TestPNGPreparation(t *testing.T) {
	t.Parallel()

	const (
		sourceWidth = 3
		middlePixel = 1
		wantedMip   = 156
		tolerance   = 1
	)

	source := image.NewNRGBA(image.Rect(0, 0, sourceWidth, 1))
	for column := range sourceWidth {
		value := byte(0)
		if column == middlePixel {
			value = 255
		}

		source.SetNRGBA(column, 0, color.NRGBA{R: value, G: value, B: value, A: 255})
	}

	data := convert(t, source, "albedo")
	if data.ColorSpace != texture.SRGB || len(data.Levels) != 2 {
		t.Fatalf("unexpected prepared texture: %+v", data)
	}
	// Every source texel contributes, and averaging occurs in linear light.
	got := int(data.Levels[1][0])
	if got < wantedMip-tolerance || got > wantedMip+tolerance {
		t.Fatalf("1x1 mip channel=%d, want about %d", got, wantedMip)
	}
}

func TestNormalMipsAreNormalized(t *testing.T) {
	t.Parallel()

	const size = 2

	source := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := range size {
		for x := range size {
			source.SetNRGBA(x, y, color.NRGBA{R: 255, G: 128, B: 255, A: 255})
		}
	}

	data := convert(t, source, "normal")
	if data.ColorSpace != texture.Linear {
		t.Fatal("normal map was encoded as sRGB")
	}

	var lengthSquared float64

	const (
		maximum     = 255
		normalRange = 2
		tolerance   = .02
	)
	for _, channel := range data.Levels[1][:3] {
		value := float64(channel)/maximum*normalRange - 1
		lengthSquared += value * value
	}

	if math.Abs(lengthSquared-1) > tolerance {
		t.Fatalf("normal mip length squared=%v", lengthSquared)
	}
}

func convert(t *testing.T, source image.Image, usage string) texture.Data {
	t.Helper()
	root := t.TempDir()
	input, output := filepath.Join(root, "source.png"), filepath.Join(root, "prepared.ktx2")

	//nolint:gosec // The fixture path is inside t.TempDir.
	file, err := os.Create(input)
	if err != nil {
		t.Fatal(err)
	}

	if err := png.Encode(file, source); err != nil {
		t.Fatal(err)
	}

	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := assetbuild.PrepareTexture(input, output, usage); err != nil {
		t.Fatal(err)
	}

	//nolint:gosec // The prepared fixture path is inside t.TempDir.
	encoded, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}

	data, err := ktx2.Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := assetbuild.PrepareTexture(input, input, usage); err == nil {
		t.Fatal("accepted input as output path")
	}

	return data
}

func TestAOMipsStayLinear(t *testing.T) {
	t.Parallel()

	const (
		width       = 3
		whiteColumn = 1
		wanted      = 85
	)

	source := image.NewNRGBA(image.Rect(0, 0, width, 1))
	for column := range width {
		value := byte(0)
		if column == whiteColumn {
			value = 255
		}

		source.SetNRGBA(column, 0, color.NRGBA{R: value, G: value, B: value, A: 255})
	}

	data := convert(t, source, "ao")
	if data.ColorSpace != texture.Linear || data.Levels[1][0] != wanted {
		t.Fatalf("AO mip should average linear occlusion: %+v", data.Levels[1])
	}
}

// Unlike RGB normals, SSBump weights must retain their magnitude through mips.
func TestSSBumpMipsPreserveVisibility(t *testing.T) {
	t.Parallel()

	const size = 2

	source := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := range size {
		for x := range size {
			source.SetNRGBA(x, y, color.NRGBA{R: 20, G: 60, B: 100, A: 255})
		}
	}

	data := convert(t, source, "ssbump")
	if data.ColorSpace != texture.Linear {
		t.Fatal("SSBump must be linear")
	}

	expected := [4]byte{20, 60, 100, 255}
	for level, pixels := range data.Levels {
		for channel, want := range expected {
			if pixels[channel] != want {
				t.Fatalf("level %d channel %d=%d, want %d", level, channel, pixels[channel], want)
			}
		}
	}
}
