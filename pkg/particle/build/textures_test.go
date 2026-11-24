//go:build !js

package build

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"slices"
	"testing"
)

func TestEmbeddedPNGPreservesColorAndTransparentGutter(t *testing.T) {
	t.Parallel()

	art := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	art.SetNRGBA(0, 0, color.NRGBA{R: 40, G: 125, B: 240, A: 64})

	sprite, err := prepareSprite("user:chip", map[string]string{"chip": pngDataURL(t, art)})
	if err != nil {
		t.Fatal(err)
	}

	const channels = 4

	page, regions := packPage([]particleSprite{sprite})
	start := page.Width * channels
	want := []byte{40, 125, 240, 0, 40, 125, 240, 64}

	if texels := page.Pixels[start : start+len(want)]; !slices.Equal(texels, want) {
		t.Fatalf("left gutter and first texel = %v, want %v", texels, want)
	}

	if aspect := regions["user:chip"].aspect; aspect != .5 {
		t.Fatalf("sprite aspect = %v, want .5", aspect)
	}
}

func TestEmbeddedPNGRejectsOversizedDimensions(t *testing.T) {
	t.Parallel()

	oversized := image.NewNRGBA(image.Rect(0, 0, builtinWidth+1, 1))

	_, err := prepareSprite("user:oversized", map[string]string{"oversized": pngDataURL(t, oversized)})
	if err == nil {
		t.Fatal("admitted a PNG larger than an atlas cell")
	}
}

func pngDataURL(t *testing.T, art image.Image) string {
	t.Helper()

	var output bytes.Buffer
	if err := png.Encode(&output, art); err != nil {
		t.Fatal(err)
	}

	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(output.Bytes())
}
