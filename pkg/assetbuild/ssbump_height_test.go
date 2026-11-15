//go:build !js

package assetbuild

import (
	"bytes"
	"math"
	"strings"
	"testing"

	"github.com/karty-game/karty-webgpu/pkg/texture"
	"github.com/karty-game/karty-webgpu/pkg/texture/ktx2"
)

func TestSSBumpAuthoredHeightRecipeAndCache(t *testing.T) {
	t.Parallel()

	project := t.TempDir()
	ssbumpTestPNG(t, project, "assets/lion-height.png", [4]byte{170, 170, 170, 255})

	config := Config{
		Input: []string{"assets/**"}, Output: "generated/assets",
		Plugins: []Plugin{Images(), SSBump(SSBumpOptions{HeightScale: 4, Rays: 16, Distance: 2}), Mipmaps(), KTX2(), GoCatalog()},
	}
	options := Options{Project: project, Identity: "height-recipe-fixture"}

	report, err := Build(t.Context(), config, options)
	if err != nil || report.Executed != 5 {
		t.Fatalf("cold height recipe: %+v %v", report, err)
	}

	encoded, err := fixtureRead(project, "generated/assets/lion-ssbump.ktx2")
	if err != nil {
		t.Fatal(err)
	}

	data, err := ktx2.Decode(encoded)
	if err != nil || data.ColorSpace != texture.Linear || len(data.Levels) != 3 ||
		!bytes.Equal(data.Levels[0][:4], []byte{85, 85, 85, 255}) {
		t.Fatalf("height coefficients/encoding changed: %+v %v", data, err)
	}

	code, err := fixtureRead(project, "generated/assets/assets.go")
	if err != nil || !bytes.Contains(code, []byte("func LionSSBump()")) || bytes.Contains(code, []byte("LionHeight")) {
		t.Fatalf("height leaked into runtime catalog: %s %v", code, err)
	}

	report, err = Build(t.Context(), config, options)
	if err != nil || report.Executed != 0 || report.Reused != 5 {
		t.Fatalf("unchanged height recipe: %+v %v", report, err)
	}

	config.Plugins[1] = SSBump(SSBumpOptions{HeightScale: 8, Rays: 16, Distance: 2})

	report, err = Build(t.Context(), config, options)
	if err != nil || report.Executed != 1 || report.Reused != 4 {
		t.Fatalf("height scale failed to invalidate bake or identical output invalidated downstream: %+v %v", report, err)
	}
}

func TestSSBumpHeightRoleAndBounds(t *testing.T) {
	t.Parallel()

	if imageUsage("tile-height") != UsageHeight || imageUsage("tile_height") != UsageHeight || imageUsage("height") != UsageHeight {
		t.Fatal("authored heights were classified as albedo")
	}

	for _, source := range []string{"tile-height.png", "tile_height.png", "height.png"} {
		if !strings.Contains(ssbumpSource(source), "ssbump") {
			t.Fatalf("height derivation = %s", ssbumpSource(source))
		}
	}

	for _, scale := range []float64{-1, math.NaN(), math.Inf(1), 513} {
		if _, err := normalizedSSBumpOptions(SSBumpOptions{HeightScale: scale}); err == nil {
			t.Fatalf("accepted scale %v", scale)
		}
	}

	height := atlasTestImage("TileHeight", UsageHeight, 4, 4, [4]byte{170, 170, 170, 255})
	if _, err := EncodeTexture(height); err == nil {
		t.Fatal("height source was uploadable without conversion")
	}

	// Linear relief reduction must retain half-height, rather than apply the
	// albedo sRGB transfer curve and inflate it to roughly 188/255.
	height.Levels[0] = bytes.Repeat([]byte{0, 0, 0, 255, 255, 255, 255, 255}, 8)
	if got := resampleImage(height, 2, 2); got[0] != 128 || got[1] != 128 || got[2] != 128 {
		t.Fatalf("height resize applied color transfer: %v", got[:4])
	}
}
