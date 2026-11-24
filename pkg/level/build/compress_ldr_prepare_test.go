//go:build !js

package levelbuild

import (
	"errors"
	"testing"

	"github.com/karty-game/ashura/internal/ktx2data"
	"github.com/karty-game/ashura/internal/leveldata"
	"github.com/karty-game/ashura/pkg/assetbuild"
	"github.com/karty-game/ashura/pkg/texture"
)

type ldrPreparationFixture struct {
	profile        NativeTextureOptions
	representation string
	observed       *bool
}

func (ldrPreparationFixture) Name() string { return "ldr-preparation-fixture" }
func (fixture ldrPreparationFixture) Plan(plan *assetbuild.Plan) error {
	source, err := plan.Add(
		assetbuild.Action{
			ID:        "source",
			Processor: "coefficients",
			Revision:  "1",
			Outputs:   []assetbuild.Slot{{Name: slot, Schema: renderPagesSchema}},
			Run:       fixture.source,
		},
	)
	if err != nil {
		return err
	}

	prepared, err := plan.Add(
		assetbuild.Action{
			ID:        "prepare",
			Processor: "ldr",
			Revision:  "1",
			Inputs:    source,
			Outputs:   []assetbuild.Slot{{Name: slot, Schema: renderPagesSchema}},
			Run: func(ctx *assetbuild.Context) (assetbuild.Result, error) {
				var page publishedPage
				if err := readRecord(ctx, source[0], &page); err != nil {
					return nil, err
				}

				if err := prepareLDRPage(ctx, &page, fixture.profile); err != nil {
					return nil, err
				}

				return result(page)
			},
		},
	)
	if err != nil {
		return err
	}

	_, err = plan.Add(
		assetbuild.Action{
			ID:        "inspect",
			Processor: "inspect",
			Revision:  "1",
			Inputs:    prepared,
			Outputs:   []assetbuild.Slot{{Name: slot, Schema: renderPagesSchema}},
			Run: func(ctx *assetbuild.Context) (assetbuild.Result, error) {
				var page publishedPage
				if err := readRecord(ctx, prepared[0], &page); err != nil {
					return nil, err
				}

				if err := fixture.inspect(ctx, page); err != nil {
					return nil, err
				}

				*fixture.observed = true

				return result(page)
			},
		},
	)

	return err
}
func (fixture ldrPreparationFixture) source(ctx *assetbuild.Context) (assetbuild.Result, error) {
	page := publishedPage{
		Page: leveldata.Page{Representation: fixture.representation, Scale: 2, Encoding: rgbeEncoding, Width: 4, Height: 4},
	}
	red := []byte{64, 128, 255}

	for index := range page.Page.MapCount() {
		pixels := make([]byte, 4*4*4)
		pixels[0], pixels[1], pixels[2], pixels[3] = red[index], 16, 8, 131

		data, err := assetbuild.EncodeTexture(
			assetbuild.Image{
				Name:   "coefficients",
				Source: "coefficients.ktx2",
				Usage:  assetbuild.UsageLightmap,
				Width:  4,
				Height: 4,
				Levels: [][]byte{pixels},
			},
		)
		if err != nil {
			return nil, err
		}

		file, err := storePublished(ctx, "lightmaps", ".ktx2", data)
		if err != nil {
			return nil, err
		}

		page.Page.Maps[index] = file.Reference
		page.Files = append(page.Files, file)
	}

	return result(page)
}
func (fixture ldrPreparationFixture) inspect(ctx *assetbuild.Context, page publishedPage) error {
	if page.Page.Encoding != fixture.profile.Transfer || page.Page.Scale <= 1 || len(page.Files) != page.Page.MapCount() {
		return ErrInvalid
	}

	previous := byte(0)

	for index := range page.Page.MapCount() {
		data, err := readPublishedLightmap(ctx, page, index)
		if err != nil {
			return err
		}

		image, err := ktx2data.Decode(data)
		if err != nil {
			return err
		}

		if image.ColorSpace != nativeTextureSpace(fixture.profile) || image.Format != texture.RGBA8 || len(image.Levels) != 1 {
			return ErrInvalid
		}

		pixels := image.Levels[0]
		if page.Page.MapCount() > 1 && pixels[0] <= previous {
			return ErrInvalid
		}

		previous = pixels[0]
		for offset := 3; offset < len(pixels); offset += 4 {
			if pixels[offset] != 255 {
				return ErrInvalid
			}
		}
	}

	if previous != 255 {
		return ErrInvalid
	}

	return nil
}
func TestLDRPagePreparationKeepsSharedScaleTransferAndOpaqueAlpha(t *testing.T) {
	t.Parallel()

	for _, representation := range []string{leveldata.RGBDiffuse, leveldata.RNMFit} {
		for _, transfer := range []string{nativeTextureLinear, nativeTextureSRGB} {
			t.Run(representation+"/"+transfer, func(t *testing.T) {
				t.Parallel()

				observed := false
				fixture := ldrPreparationFixture{
					profile:        ldrProfile(t, "", "", transfer),
					representation: representation,
					observed:       &observed,
				}

				_, err := assetbuild.Build(
					t.Context(),
					assetbuild.Config{Output: "generated/assets",
						Pipelines: []assetbuild.Pipeline{{Name: "fixture",
							Import: []assetbuild.Plugin{fixture},
						}},
					},
					assetbuild.Options{Project: t.TempDir(), Identity: "ldr-preparation", NoCache: true},
				)
				if err != nil || !observed {
					t.Fatal("incoherent LDR preparation", err, observed)
				}
			})
		}
	}
}

func TestLDRPreparationRejectsUncalibratedOrNonProductionPagesBeforeIO(t *testing.T) {
	t.Parallel()

	for _, page := range []leveldata.Page{
		{Encoding: rgbeEncoding},
		{Encoding: rgbeEncoding, Representation: "unknown"},
		{Encoding: nativeTextureLinear, Representation: leveldata.RGBDiffuse},
	} {
		if err := prepareLDRPage(nil, &publishedPage{Page: page}, NativeTextureOptions{}); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid source reached coefficient I/O", page, err)
		}
	}
}
