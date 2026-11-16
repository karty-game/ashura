//go:build !js

package levelbuild

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/karty-game/karty-webgpu/internal/leveldata"
	"github.com/karty-game/karty-webgpu/internal/modeldata"
	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
)

type separateNativeVariants struct{ profiles []PublishOptions }

func (separateNativeVariants) Name() string { return "separate-native-variants" }
func (fixture separateNativeVariants) Plan(plan *assetbuild.Plan) error {
	files := make(map[string]fileRecord)
	codecs := &codecActionCache{}
	atlases := &atlasActionCache{}
	pages := &packageActionCache{}

	for _, profile := range fixture.profiles {
		profile.sharedFiles, profile.sharedCodecs = files, codecs

		profile.sharedAtlases, profile.sharedPages = atlases, pages
		if err := Publish(profile).Plan(plan); err != nil {
			return err
		}
	}

	return nil
}

func variantFixtureProfiles() []PublishOptions {
	quality := 0
	profile := func(name, family, representation string) PublishOptions {
		return PublishOptions{
			Variant: name, Representation: representation,
			NativeTextures: &NativeTextureOptions{
				Family: family, NativeAlbedo: true,
				Encoder: assetbuild.BlockCompressionOptions{LDRQuality: &quality, SSBumpQuality: &quality},
			},
			AlbedoETC1S: &ETC1SOptions{Quality: 192},
		}
	}

	return []PublishOptions{
		profile("RGBBC", "bc", "rgb-diffuse-v1"), profile("RGBMobile", "etc2", "rgb-diffuse-v1"),
		profile("RNMBC", "bc", "rnm-fit-v1"), profile("RNMMobile", "etc2", "rnm-fit-v1"),
	}
}

func TestNativeVariantBatchRetainsEmptyLevelProfilesWithoutTools(t *testing.T) {
	t.Parallel()

	profiles := variantFixtureProfiles()

	var previous map[string]string

	for _, batch := range []bool{false, true} {
		project := t.TempDir()

		var publisher assetbuild.Plugin = separateNativeVariants{profiles: profiles}
		if batch {
			publisher = PublishVariants(profiles...)
		}

		config := assetbuild.Config{Output: "generated", Pipelines: []assetbuild.Pipeline{{Name: "fixture",
			Import: []assetbuild.Plugin{emptyPublishedLevel{}, publisher},
		}}}
		if _, err := assetbuild.Build(t.Context(), config, assetbuild.Options{Project: project, Jobs: 4, NoCache: true}); err != nil {
			t.Fatal(err)
		}

		current := materialOutputFiles(t, filepath.Join(project, "generated"))
		if batch && !reflect.DeepEqual(previous, current) {
			t.Fatal("batched publication changed empty-level profile manifests or descriptors")
		}

		previous = current
	}
}

//nolint:paralleltest // Four complete codec builds deliberately bound VM memory.
func TestPortableVariantBatchMatchesCompletePreparedPayloads(t *testing.T) {
	profiles := variantFixtureProfiles()

	var previous map[string]string

	for _, batch := range []bool{false, true} {
		project := writeNativeVariantFixture(t)

		var publisher assetbuild.Plugin = separateNativeVariants{profiles: profiles}
		if batch {
			publisher = PublishVariants(profiles...)
		}

		config := fixtureConfig(syntheticBake())
		config.Pipelines[0].Process = append(config.Pipelines[0].Process, fixtureProductionMaps())
		config.Pipelines[0].Output = []assetbuild.Plugin{publisher}

		options := assetbuild.Options{Project: project, Jobs: 4, NoCache: true}
		if _, err := assetbuild.Build(t.Context(), config, options); err != nil {
			t.Fatal(err)
		}

		current := make(map[string]string)

		for _, directory := range []string{"public", "levels"} {
			for name, contents := range materialOutputFiles(t, filepath.Join(project, "generated", directory)) {
				current[directory+name] = contents
			}
		}

		if batch && !reflect.DeepEqual(previous, current) {
			t.Fatal("batched codec graph changed complete geometry/UV/textures/manifests/descriptors")
		}

		previous = current

		if batch {
			options.NoCache = false
			// Populate and then verify the normal admitted cache, including the
			// native-albedo producers that now consume declared ETC1S references.
			if _, err := assetbuild.Build(t.Context(), config, options); err != nil {
				t.Fatal(err)
			}

			if report, err := assetbuild.Build(t.Context(), config, options); err != nil || report.Executed != 0 {
				t.Fatal("unchanged batched publication did not reuse complete admitted producers", report, err)
			}
		}
	}
}

func writeNativeVariantFixture(t *testing.T) string {
	t.Helper()
	project := writeFixture(t)
	path := filepath.Join(project, "floor.kmodel")
	//nolint:gosec // The fixed floor.kmodel name stays inside this test's private TempDir.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	document, err := modeldata.Decode(data)
	if err != nil {
		t.Fatal(err)
	}

	image, err := assetbuild.GenerateMipmaps(assetbuild.Image{
		Name: "Albedo", Source: "floor.png", Usage: assetbuild.UsageAlbedo, Width: 4, Height: 4,
		Levels: [][]byte{bytes.Repeat([]byte{200, 180, 160, 255}, 16)},
	})
	if err != nil {
		t.Fatal(err)
	}

	document.Textures[0].Data, err = assetbuild.EncodeTexture(image)
	if err != nil {
		t.Fatal(err)
	}

	data, err = modeldata.Encode(document)
	if err != nil {
		t.Fatal(err)
	}
	//nolint:gosec // The fixed floor.kmodel name stays inside this test's private TempDir.
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	return project
}

func fixtureProductionMaps() assetbuild.Plugin {
	return stage{
		name:   "fixture-production-maps",
		schema: bakedSchema,
		run: func(plan *assetbuild.Plan, resource assetbuild.Resource) error {
			refs, err := plan.Add(assetbuild.Action{
				ID: "fixture-production-maps", Processor: "fixture-production-maps", Revision: "1",
				Inputs: []assetbuild.Ref{resource.Ref}, Outputs: []assetbuild.Slot{{Name: slot, Schema: bakedSchema}},
				Run: func(ctx *assetbuild.Context) (assetbuild.Result, error) {
					var record partitioned
					if err := readRecord(ctx, resource.Ref, &record); err != nil {
						return nil, err
					}

					for chunk := range record.Chunks {
						for page := range record.Chunks[chunk].Pages {
							entry := &record.Chunks[chunk].Pages[page]
							entry.Representation, entry.Encoding = leveldata.RNMFit, rgbeEncoding
							entry.RGBMap = entry.Maps[0]
						}
					}

					return result(record)
				},
			})
			if err == nil {
				replaceResource(plan, resource.ID, refs[0])
			}

			return err
		},
	}
}
