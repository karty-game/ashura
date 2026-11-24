//go:build !js

package levelbuild

import (
	"bytes"
	"strings"
	"testing"

	"github.com/karty-game/ashura/internal/leveldata"
	"github.com/karty-game/ashura/pkg/assetbuild"
)

// fixtureChunkBundleBake authors a constant physical diffuse/pi RGB image.
// It deliberately does not relabel the old synthetic legacy RNM coefficients.
func fixtureChunkBundleBake() assetbuild.Plugin {
	return stage{name: "fixture-bundle-bake", schema: uvSchema, run: func(plan *assetbuild.Plan, resource assetbuild.Resource) error {
		refs, err := plan.Add(assetbuild.Action{
			ID: "fixture/bundle-bake/" + resource.ID, Processor: "fixture-bundle-bake", Revision: "1",
			Inputs: []assetbuild.Ref{resource.Ref}, Outputs: []assetbuild.Slot{{Name: slot, Schema: bakedSchema}},
			Run: func(ctx *assetbuild.Context) (assetbuild.Result, error) {
				var record partitioned
				if err := readRecord(ctx, resource.Ref, &record); err != nil {
					return nil, err
				}

				for ci := range record.Chunks {
					for pi := range record.Chunks[ci].Pages {
						page := &record.Chunks[ci].Pages[pi]

						encoded, err := assetbuild.EncodeTexture(assetbuild.Image{
							Name: "Diffuse", Source: "diffuse.png", Usage: assetbuild.UsageLightmap,
							Width: int(page.Width), Height: int(page.Height),
							Levels: [][]byte{bytes.Repeat([]byte{128, 96, 64, 128}, int(page.Width*page.Height))},
						})
						if err != nil {
							return nil, err
						}

						artifact, err := ctx.StoreArtifact(encoded)
						if err != nil {
							return nil, err
						}

						page.RGBMap, page.Maps = artifact, [3]assetbuild.Artifact{artifact}
						page.Representation, page.Encoding, page.Scale = leveldata.RGBDiffuse, rgbeEncoding, 1
					}
				}

				return result(record)
			},
		})
		if err == nil {
			replaceResource(plan, resource.ID, refs[0])
		}

		return err
	}}
}

func TestChunkBundleDirectoryUsesNativeFamilyInsteadOfVariant(t *testing.T) {
	t.Parallel()

	for _, fixture := range []struct {
		options PublishOptions
		want    string
	}{
		{PublishOptions{Variant: "BC"}, "levels/chunks"},
		{PublishOptions{Variant: "Mobile", NativeTextures: &NativeTextureOptions{}}, "texture-profiles/bc/levels/chunks"},
		{PublishOptions{Variant: "Desktop", NativeTextures: &NativeTextureOptions{Family: nativeTextureETC2}}, "texture-profiles/mobile/levels/chunks"},
		{PublishOptions{NativeHDR: &NativeHDROptions{}}, "texture-profiles/bc/levels/chunks"},
		{PublishOptions{NativeHDR: &NativeHDROptions{Family: nativeHDRMobileFamily}}, "texture-profiles/mobile/levels/chunks"},
	} {
		if got := chunkBundleDirectory(fixture.options); got != fixture.want {
			t.Fatal(got, fixture.want)
		}
	}
}

func TestChunkBundlesRejectLegacyUncalibratedPages(t *testing.T) {
	t.Parallel()
	project := writeFixture(t)
	config := fixtureConfig(syntheticBake())
	config.Pipelines[0].Output = []assetbuild.Plugin{Publish(PublishOptions{BundleChunks: true})}

	_, err := assetbuild.Build(t.Context(), config, assetbuild.Options{Project: project, Identity: "legacy-chunk-bundle"})
	if err == nil || !strings.Contains(err.Error(), "chunk bundles require calibrated RGB or RNM pages") {
		t.Fatal("uncalibrated legacy pages must not be silently relabeled", err)
	}
}
