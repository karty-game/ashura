//go:build !js

package assetbuild

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"strings"
	"testing"

	"github.com/karty-game/ashura/pkg/texture"
	"github.com/karty-game/ashura/pkg/texture/ktx2"
)

func TestSSBumpDirectionsIgnoreUnrelatedNormalAlpha(t *testing.T) {
	t.Parallel()

	options, err := normalizedSSBumpOptions(SSBumpOptions{Rays: 8, Distance: 2})
	if err != nil {
		t.Fatal(err)
	}

	opaque := atlasTestImage("TileNormal", UsageNormal, 4, 4, [4]byte{128, 128, 255, 255})
	opaque.Source = "tile-normal.png"
	transparent := atlasTestImage("TileNormal", UsageNormal, 4, 4, [4]byte{128, 128, 255, 32})
	transparent.Source = opaque.Source

	first, err := bakeSSBumpImage(t.Context(), opaque, options)
	if err != nil {
		t.Fatal(err)
	}

	second, err := bakeSSBumpImage(t.Context(), transparent, options)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(first.Levels[0], second.Levels[0]) || transparent.Levels[0][3] != 32 {
		t.Fatal("normal alpha changed baked directions or the immutable input")
	}
}

func TestSSBumpPluginColdWarmAndOptions(t *testing.T) {
	t.Parallel()
	project := t.TempDir()
	ssbumpTestPNG(t, project, "assets/brick-normal.png", [4]byte{128, 128, 255, 255})
	ssbumpTestPNG(t, project, "assets/brick-albedo.png", [4]byte{160, 40, 10, 255})

	config := Config{
		Input: []string{"assets/**"}, Output: "generated/assets",
		Plugins: []Plugin{Images(), SSBump(SSBumpOptions{Include: []string{"*-normal"}}), Mipmaps(), KTX2(), GoCatalog()},
	}
	options := Options{Project: project, Identity: "ssbump-cache-fixture"}

	report, err := Build(t.Context(), config, options)
	if err != nil || report.Executed != 8 {
		t.Fatalf("cold SSBump build: %+v %v", report, err)
	}

	prepared, err := fixtureRead(project, "generated/assets/brick-ssbump.ktx2")
	if err != nil {
		t.Fatal(err)
	}

	data, err := ktx2.Decode(prepared)
	if err != nil {
		t.Fatal(err)
	}

	if data.Width != 4 || data.Height != 4 || len(data.Levels) != 3 || data.ColorSpace != texture.Linear {
		t.Fatalf("SSBump must be linear with authored mip chain: %+v", data)
	}

	code, err := fixtureRead(project, "generated/assets/assets.go")
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Contains(code, []byte("func BrickSSBump()")) || !bytes.Contains(code, []byte("func BrickAlbedo()")) ||
		bytes.Contains(code, []byte("func BrickNormal()")) {
		t.Fatalf("unexpected derived declarations: %s", code)
	}

	report, err = Build(t.Context(), config, options)
	if err != nil || report.Executed != 0 || report.Reused != 8 {
		t.Fatalf("warm SSBump build: %+v %v", report, err)
	}

	config.Plugins[1] = SSBump(SSBumpOptions{Include: []string{"*-normal"}, Rays: 16})

	report, err = Build(t.Context(), config, options)
	if err != nil || report.Executed != 1 || report.Reused != 7 {
		t.Fatalf("changed rays with identical flat result must stop downstream invalidation: %+v %v", report, err)
	}

	after, err := fixtureRead(project, "generated/assets/brick-ssbump.ktx2")
	if err != nil || !bytes.Equal(prepared, after) {
		t.Fatalf("flat visibility coefficients changed with ray count: %v", err)
	}
}

func TestSSBumpGroupedMapsAndKeepNormals(t *testing.T) {
	t.Parallel()

	albedo := atlasTestImage("BrickAlbedo", UsageAlbedo, 4, 4, [4]byte{160, 40, 10, 255})
	normal := atlasTestImage("BrickNormal", UsageNormal, 4, 4, [4]byte{128, 128, 255, 255})
	normal.Source = "brick-normal.png"
	occlusion := atlasTestImage("BrickAO", UsageAO, 4, 4, [4]byte{100, 100, 100, 255})
	input := ImageSet{Images: []Image{albedo, normal, occlusion}}

	for _, keep := range []bool{false, true} {
		t.Run(map[bool]string{false: "replace", true: "retain"}[keep], func(t *testing.T) {
			t.Parallel()

			options, err := normalizedSSBumpOptions(SSBumpOptions{Rays: 8, Distance: 1, KeepNormals: keep})
			if err != nil {
				t.Fatal(err)
			}

			result, err := bakeSSBumpSet(t.Context(), input, options)
			if err != nil {
				t.Fatal(err)
			}

			expected := 3
			if keep {
				expected++
			}

			if len(result.Images) != expected {
				t.Fatalf("got %d maps, want %d", len(result.Images), expected)
			}

			found := make(map[string]Image)
			for _, value := range result.Images {
				found[value.Name] = value
			}

			if !bytes.Equal(found[albedo.Name].Levels[0], albedo.Levels[0]) ||
				!bytes.Equal(found[occlusion.Name].Levels[0], occlusion.Levels[0]) {
				t.Fatal("normal conversion changed nonnormal maps")
			}

			derived := found["BrickSSBump"]
			if derived.Usage != UsageSSBump || derived.Source != "brick-ssbump.png" || len(derived.Levels) != 1 {
				t.Fatalf("wrong derived metadata: %+v", derived)
			}

			_, retained := found[normal.Name]
			if retained != keep {
				t.Fatalf("normal retained=%t, want %t", retained, keep)
			}
		})
	}
}

func TestSSBumpSourceIdentities(t *testing.T) {
	t.Parallel()

	for source, expected := range map[string]string{
		"brick-normal.png":      "brick-ssbump.png",
		"tile/brick_normal.jpg": "tile/brick_ssbump.jpg",
		"tile/normal.png":       "tile/ssbump.png",
		"tile/BRICK-NORMAL.PNG": "tile/BRICK-ssbump.PNG",
		"tile.v1-normal":        "tile.v1-ssbump",
		"detail.png":            "detail-ssbump.png",
	} {
		if got := ssbumpSource(source); got != expected {
			t.Fatalf("source %s -> %s, want %s", source, got, expected)
		}
	}

	options, err := normalizedSSBumpOptions(SSBumpOptions{Rays: 8, Distance: 1})
	if err != nil {
		t.Fatal(err)
	}

	normal := atlasTestImage("TileV1Normal", UsageNormal, 4, 4, [4]byte{128, 128, 255, 255})
	normal.Source = "tile.v1-normal"

	derived, err := bakeSSBumpImage(t.Context(), normal, options)
	if err != nil || derived.Name != "TileV1SSBump" || derived.Source != "tile.v1-ssbump" {
		t.Fatalf("dotted synthetic identity was truncated: %+v %v", derived, err)
	}
}

func TestSSBumpRejectsInvalidInputAndCancellation(t *testing.T) {
	t.Parallel()

	options, err := normalizedSSBumpOptions(SSBumpOptions{Rays: 8, Distance: 1})
	if err != nil {
		t.Fatal(err)
	}

	for name, input := range map[string]Image{
		"rectangular":      atlasTestImage("Normal", UsageNormal, 4, 2, [4]byte{128, 128, 255, 255}),
		"non-power-of-two": atlasTestImage("Normal", UsageNormal, 3, 3, [4]byte{128, 128, 255, 255}),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if result, err := bakeSSBumpSet(t.Context(), ImageSet{Images: []Image{input}}, options); err == nil || len(result.Images) != 0 {
				t.Fatalf("invalid normal produced fallback output: %+v %v", result, err)
			}
		})
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	input := atlasTestImage("Normal", UsageNormal, 4, 4, [4]byte{128, 128, 255, 255})
	if _, err := bakeSSBumpSet(ctx, ImageSet{Images: []Image{input}}, options); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled bake returned %v", err)
	}

	input.Levels = append(input.Levels, make([]byte, 2*2*imageChannels))
	if _, err := bakeSSBumpSet(
		t.Context(),
		ImageSet{Images: []Image{input}},
		options,
	); err == nil ||
		!strings.Contains(err.Error(), "one unpacked level") {
		t.Fatalf("pre-mipped normal was not rejected: %v", err)
	}
}

func ssbumpTestPNG(t *testing.T, project, name string, pixel [4]byte) {
	t.Helper()

	data := atlasTestImage("Fixture", UsageAlbedo, 4, 4, pixel).Levels[0]
	source := &image.NRGBA{Pix: data, Stride: 4 * imageChannels, Rect: image.Rect(0, 0, 4, 4)}

	var buffer bytes.Buffer

	if err := png.Encode(&buffer, source); err != nil {
		t.Fatal(err)
	}

	fixtureWrite(t, project, name, buffer.Bytes())
}
