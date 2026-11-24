//go:build !js

package levelbuild

import (
	"bytes"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/karty-game/ashura/internal/ktx2data"
	"github.com/karty-game/ashura/internal/modeldata"
	"github.com/karty-game/ashura/internal/texturedata"
	"github.com/karty-game/ashura/pkg/assetbuild"
)

func sourceNormalPayload(t *testing.T, width, height int, pixel [4]byte, mipmaps bool) []byte {
	t.Helper()

	value := assetbuild.Image{Name: "Normal", Source: "normal.png", Usage: assetbuild.UsageNormal, Width: width, Height: height,
		Levels: [][]byte{bytes.Repeat(pixel[:], width*height)}}

	if mipmaps {
		var err error

		value, err = assetbuild.GenerateMipmaps(value)
		if err != nil {
			t.Fatal(err)
		}
	}

	data, err := assetbuild.EncodeTexture(value)
	if err != nil {
		t.Fatal(err)
	}

	return data
}

func TestSourceSSBumpProjectsNormalStrengthOffline(t *testing.T) {
	t.Parallel()
	payload := sourceNormalPayload(t, 4, 4, [4]byte{191, 128, 238, 0}, false)

	flat, err := bakeSourceSSBump(t.Context(), payload, 0)
	if err != nil {
		t.Fatal(err)
	}

	tilted, err := bakeSourceSSBump(t.Context(), payload, 1)
	if err != nil {
		t.Fatal(err)
	}

	flatData, err := ktx2data.Decode(flat)
	if err != nil {
		t.Fatal(err)
	}

	tiltedData, err := ktx2data.Decode(tilted)
	if err != nil {
		t.Fatal(err)
	}

	if flatData.ColorSpace != texturedata.Linear || len(flatData.Levels) != 3 {
		t.Fatalf("prepared profile %+v", flatData)
	}

	for _, channel := range flatData.Levels[0][:3] {
		if math.Abs(float64(channel)-85) > 3 {
			t.Fatalf("flat coefficient %d", channel)
		}
	}

	if tiltedData.Levels[0][0] <= flatData.Levels[0][0]+30 || tiltedData.Levels[0][1] >= flatData.Levels[0][1]-10 {
		t.Fatalf("lost normal response flat%v tilted%v", flatData.Levels[0][:4], tiltedData.Levels[0][:4])
	}

	if tiltedData.Levels[0][3] != 255 {
		t.Fatal("numeric alpha attenuated normals")
	}

	negative := sourceNormalPayload(t, 2, 2, [4]byte{128, 128, 0, 255}, false)

	flatNegative, err := bakeSourceSSBump(t.Context(), negative, 0)
	if err != nil {
		t.Fatal(err)
	}

	negativeData, err := ktx2data.Decode(flatNegative)
	if err != nil {
		t.Fatal(err)
	}

	for _, channel := range negativeData.Levels[0][:3] {
		if math.Abs(float64(channel)-85) > 3 {
			t.Fatalf("disabled normal kept negativeZ: %v", negativeData.Levels[0][:4])
		}
	}
}

func TestSourceNormalMipCapAndDimensions(t *testing.T) {
	t.Parallel()

	for _, mipmaps := range []bool{false, true} {
		payload := sourceNormalPayload(t, 1024, 1024, [4]byte{128, 128, 255, 255}, mipmaps)

		image, err := sourceNormalImage(payload, 1)
		if err != nil {
			t.Fatal(err)
		}

		if image.Bounds().Dx() != 512 || image.Bounds().Dy() != 512 {
			t.Fatalf("normal cap %v", image.Bounds())
		}
	}

	constant := sourceNormalPayload(t, 1, 1, [4]byte{128, 128, 255, 255}, false)

	image, err := sourceNormalImage(constant, 1)
	if err != nil || image.Bounds().Dx() != 2 {
		t.Fatalf("constant normal: %v %v", image, err)
	}

	for _, size := range [][2]int{{16, 8}, {3, 3}} {
		if _, err := sourceNormalImage(sourceNormalPayload(t, size[0], size[1], [4]byte{128, 128, 255, 255}, false), 1); err == nil {
			t.Fatalf("unsupported dimensions %v admitted", size)
		}
	}
}

type sourceTexturePlugin struct {
	run func(*assetbuild.Context) error
}

func (sourceTexturePlugin) Name() string { return "source-texture-fixture" }
func (plugin sourceTexturePlugin) Plan(plan *assetbuild.Plan) error {
	_, err := plan.Add(
		assetbuild.Action{
			ID:        "source-texture-fixture",
			Processor: "source-texture-fixture",
			Revision:  "1",
			Outputs:   []assetbuild.Slot{{Name: "done", Schema: "test@1"}},
			Run: func(ctx *assetbuild.Context) (assetbuild.Result, error) {
				return assetbuild.Result{"done": nil}, plugin.run(ctx)
			},
		},
	)

	return err
}

func TestAutoSSBumpPreservesOverridesAndDeduplicates(t *testing.T) {
	t.Parallel()
	payload := sourceNormalPayload(t, 4, 4, [4]byte{191, 128, 238, 255}, false)
	doc := modeldata.Document{Textures: []modeldata.Texture{{Role: modeldata.Normal, Data: payload}}, Parts: []modeldata.Part{
		{Name: "first", Material: modeldata.Material{Normal: 0, NormalStrength: 1}},
		{Name: "second", Material: modeldata.Material{Normal: 0, NormalStrength: 1}},
		{Name: "flat", Material: modeldata.Material{Normal: 0, NormalStrength: 0}},
		{Name: "authored", Material: modeldata.Material{Normal: 0, NormalStrength: 1}},
	}}

	var (
		result   map[string]assetbuild.Artifact
		authored assetbuild.Artifact
	)

	project := t.TempDir()

	config := assetbuild.Config{Output: "generated/assets",
		Pipelines: []assetbuild.Pipeline{{Name: "fixture",
			Import: []assetbuild.Plugin{sourceTexturePlugin{run: func(ctx *assetbuild.Context) error {
				var err error

				authored, err = ctx.StoreArtifact([]byte("authored"))
				if err != nil {
					return err
				}

				explicit := map[string]assetbuild.Artifact{"authored": authored}

				result, err = autoSSBump(ctx, doc, explicit)
				if err == nil {
					result["extra"] = authored

					if _, exists := explicit["extra"]; exists {
						t.Fatal("mutated override map")
					}
				}

				return err
			}}},
		}},
	}
	if _, err := assetbuild.Build(t.Context(), config, assetbuild.Options{Project: project, Identity: "normal-to-source"}); err != nil {
		t.Fatal(err)
	}

	if result["first"] != result["second"] || result["first"] == result["flat"] || result["authored"] != authored {
		t.Fatalf("normal identities %+v", result)
	}
	//nolint:gosec // Fixture temp project plus the validated artifact producer SHA, never an authored path.
	data, err := os.ReadFile(filepath.Join(project, "generated/cache/assets/v1/artifacts", result["first"].SHA256))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := ktx2data.Decode(data); err != nil {
		t.Fatalf("published derived payload: %v", err)
	}
}

func TestSourceTextureProfileAdmission(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name  string
		usage string
		width int
		space texturedata.ColorSpace
		valid bool
	}{
		{name: "albedo", usage: assetbuild.UsageAlbedo, width: 2, space: texturedata.SRGB, valid: true},
		{name: "ssbump", usage: assetbuild.UsageSSBump, width: 2, space: texturedata.Linear, valid: true},
		{name: "albedo-as-numeric", usage: assetbuild.UsageAlbedo, width: 2, space: texturedata.Linear},
		{name: "numeric-as-albedo", usage: assetbuild.UsageSSBump, width: 2, space: texturedata.SRGB},
		{name: "oversized", usage: assetbuild.UsageSSBump, width: 2048, space: texturedata.Linear},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			data, err := assetbuild.EncodeTexture(assetbuild.Image{Name: "Profile", Source: "profile.png", Usage: test.usage,
				Width: test.width, Height: test.width, Levels: [][]byte{bytes.Repeat([]byte{85, 85, 85, 255}, test.width*test.width)}})
			if err != nil {
				t.Fatal(err)
			}

			config := assetbuild.Config{Output: "generated/assets",
				Pipelines: []assetbuild.Pipeline{{Name: "fixture",
					Import: []assetbuild.Plugin{sourceTexturePlugin{run: func(ctx *assetbuild.Context) error {
						_, err := storeSourceTexture(ctx, data, test.space)

						return err
					}}},
				}},
			}

			_, err = assetbuild.Build(t.Context(), config, assetbuild.Options{Project: t.TempDir(), Identity: test.name})
			if (err == nil) != test.valid {
				t.Fatalf("profile valid=%t error%v", test.valid, err)
			}
		})
	}
}

func TestAutoSSBumpRejectsUnknownPrimitiveOverride(t *testing.T) {
	t.Parallel()
	ctx := &assetbuild.Context{Context: t.Context()}

	doc := modeldata.Document{Parts: []modeldata.Part{{Name: "renamed", Material: modeldata.Material{Normal: -1}}}}
	if _, err := autoSSBump(ctx, doc, map[string]assetbuild.Artifact{"original": {}}); err == nil {
		t.Fatal("renamed primitive silently dropped its authored relief override")
	}
}
