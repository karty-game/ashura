//go:build !js

package levelbuild

import (
	"bytes"
	"testing"

	"github.com/karty-game/karty-webgpu/internal/ktx2data"
	"github.com/karty-game/karty-webgpu/internal/leveldata"
	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
	"github.com/karty-game/karty-webgpu/pkg/texture"
)

type ldrSSBumpPreservationFixture struct {
	source   []byte
	observed *bool
}

func (ldrSSBumpPreservationFixture) Name() string { return "ldr-ssbump-preservation" }

func (fixture ldrSSBumpPreservationFixture) Plan(plan *assetbuild.Plan) error {
	source, err := plan.Add(assetbuild.Action{
		ID: "source", Processor: "authored-ssbump", Revision: "1",
		Outputs: []assetbuild.Slot{{Name: slot, Schema: "karty.native-block-file@1"}},
		Run: func(ctx *assetbuild.Context) (assetbuild.Result, error) {
			file, err := storePublished(ctx, "textures", ".ktx2", fixture.source)
			if err != nil {
				return nil, err
			}

			return result(file)
		},
	})
	if err != nil {
		return err
	}

	return plan.Expand(assetbuild.Expansion{
		ID: "portable-ssbump", Inputs: source,
		Run: func(ctx *assetbuild.Context, child *assetbuild.Plan) error {
			var file fileRecord
			if err := readRecord(ctx, source[0], &file); err != nil {
				return err
			}

			encoded, err := addLDRRawSSBumpAction(child, "fixture", "ssbump", file, nil)
			if err != nil {
				return err
			}

			_, err = child.Add(assetbuild.Action{
				ID: "inspect", Processor: "inspect-exact-ssbump", Revision: "1",
				Inputs:  []assetbuild.Ref{encoded},
				Outputs: []assetbuild.Slot{{Name: slot, Schema: "karty.native-block-file@1"}},
				Run: func(ctx *assetbuild.Context) (assetbuild.Result, error) {
					return fixture.inspect(ctx, file, encoded)
				},
			})

			return err
		},
	})
}

func (fixture ldrSSBumpPreservationFixture) inspect(
	ctx *assetbuild.Context,
	source fileRecord,
	encoded assetbuild.Ref,
) (assetbuild.Result, error) {
	var output fileRecord
	if err := readRecord(ctx, encoded, &output); err != nil {
		return nil, err
	}

	original, err := ctx.ReadArtifact(source.Artifact)
	if err != nil || !bytes.Equal(original, fixture.source) {
		return nil, ErrInvalid
	}

	page := publishedPage{Page: leveldata.Page{
		Representation: leveldata.RGBDiffuse,
		Maps:           [3]leveldata.Ref{source.Reference},
		Materials:      []leveldata.Material{{Name: "aliased", SSBump: &source.Reference}},
	}, Files: []fileRecord{source}}
	if err := replaceNativePageFiles(ctx, &page, map[string]assetbuild.Ref{
		nativeBlockKey(nativeRNMRole, source.Reference.SHA256):    encoded,
		nativeBlockKey(nativeSSBumpRole, source.Reference.SHA256): encoded,
	}); err != nil || len(page.Files) == 0 {
		return nil, ErrInvalid
	}

	for _, file := range page.Files {
		if file != output {
			return nil, ErrInvalid
		}
	}

	canonical, err := ctx.ReadArtifact(output.Artifact)
	if err != nil {
		return nil, err
	}

	before, err := ktx2data.Decode(original)
	if err != nil {
		return nil, err
	}

	after, err := ktx2data.Decode(canonical)
	if err != nil || after.Format != texture.RGBA8 || after.ColorSpace != texture.Linear ||
		after.Width != before.Width || after.Height != before.Height || len(after.Levels) != len(before.Levels) {
		return nil, ErrInvalid
	}

	for index := range before.Levels {
		if !bytes.Equal(before.Levels[index], after.Levels[index]) {
			return nil, ErrInvalid
		}
	}

	*fixture.observed = true

	return result(output)
}

func TestPortableSSBumpPublisherRetainsAuthoredSamplesWithoutTools(t *testing.T) {
	t.Parallel()

	const width = 4

	levels := [][]byte{
		bytes.Repeat([]byte{128, 65, 33, 17}, width*width),
		bytes.Repeat([]byte{31, 129, 67, 0}, width),
		{7, 11, 19, 255},
	}

	data, err := assetbuild.EncodeTexture(assetbuild.Image{
		Name: "Authored", Source: "authored.ktx2", Usage: assetbuild.UsageSSBump,
		Width: width, Height: width, Levels: levels,
	})
	if err != nil {
		t.Fatal(err)
	}

	canonical, err := assetbuild.PrepareNativeSSBumpRGBA8(data)
	if err != nil {
		t.Fatal(err)
	}

	for name, source := range map[string][]byte{"authored": data, "already-canonical": canonical} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			observed := false

			_, err := assetbuild.Build(t.Context(), assetbuild.Config{Output: "generated/assets",
				Pipelines: []assetbuild.Pipeline{{Name: "fixture",
					Import: []assetbuild.Plugin{ldrSSBumpPreservationFixture{source: source, observed: &observed}},
				}},
			}, assetbuild.Options{Project: t.TempDir(), Identity: "portable-authored-ssbump", NoCache: true})
			if err != nil || !observed {
				t.Fatal("portable authored coefficients changed", err, observed)
			}
		})
	}
}
