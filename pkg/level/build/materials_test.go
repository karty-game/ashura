//go:build !js

package levelbuild

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
)

func TestStagedMaterialsMatchEncodedInputsWithoutStandaloneTextures(t *testing.T) {
	t.Parallel()
	encodedProject, stagedProject := writeFixture(t), writeFixture(t)
	writeMixedMaskFixture(t, encodedProject)
	writeMixedMaskFixture(t, stagedProject)
	writeReliefFixture(t, encodedProject, 17)
	writeReliefFixture(t, stagedProject, 17)

	encoded, staged := materialFixtureConfig(false), materialFixtureConfig(true)
	for index, project := range []string{encodedProject, stagedProject} {
		config := []assetbuild.Config{encoded, staged}[index]
		if _, err := assetbuild.Build(t.Context(), config, assetbuild.Options{Project: project, Identity: "staged-materials"}); err != nil {
			t.Fatal(err)
		}
	}

	for _, directory := range []string{"public", "scenes"} {
		before := materialOutputFiles(t, filepath.Join(encodedProject, "generated", directory))

		after := materialOutputFiles(t, filepath.Join(stagedProject, "generated", directory))
		if !reflect.DeepEqual(before, after) {
			t.Fatal("scene/material split changed published runtime or factory bytes", directory)
		}
	}

	if _, err := os.Stat(filepath.Join(encodedProject, "generated/floor-ssbump.ktx2")); err != nil {
		t.Fatal("encoded fixture did not exercise standalone texture publication", err)
	}

	if _, err := os.Stat(filepath.Join(stagedProject, "generated/floor-ssbump.ktx2")); !os.IsNotExist(err) {
		t.Fatal("material intermediate leaked into project output", err)
	}
	// The split must preserve the existing opaque/cutout separation and coverage.
	manifest := findManifest(t, stagedProject)
	for _, chunk := range manifest.Chunks {
		for _, page := range chunk.Pages {
			for _, part := range readPreparedModel(t, stagedProject, page.Geometry).Parts {
				if part.Material.AlphaMask {
					t.Fatal("cutout entered the opaque bake")
				}
			}
		}
	}

	writeReliefFixture(t, stagedProject, 43)

	report, err := assetbuild.Build(t.Context(), staged, assetbuild.Options{Project: stagedProject, Identity: "staged-materials"})
	if err != nil {
		t.Fatal(err)
	}

	var imported, processed bool

	for _, action := range report.Actions {
		if action.ID == "pipeline/fixture/level/source/test.scene.yaml" {
			imported = action.Status == "HIT"
		}

		if action.ID == "pipeline/fixture/level/materials/test.scene.yaml" {
			processed = action.Status == "MISS"
		}
	}

	if !imported || !processed {
		t.Fatal("relief edit must reuse scene import and invalidate material processing", report)
	}
}

func TestStagedMaterialsRejectMissingProcessingAndUnknownBindings(t *testing.T) {
	t.Parallel()

	for _, scenario := range []string{"missing-materials", "unprocessed-height", "unknown-primitive"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			project := writeFixture(t)
			writeReliefFixture(t, project, 17)

			config := materialFixtureConfig(true)

			switch scenario {
			case "missing-materials":
				config.Pipelines[0].Process = config.Pipelines[0].Process[3:]
			case "unprocessed-height":
				config.Pipelines[0].Process = config.Pipelines[0].Process[2:]
			case "unknown-primitive":
				config.Pipelines[0].Process[2] = Materials(
					MaterialOptions{SSBump: map[string]map[string]string{"floor.glb": {"renamed-floor": "floor-height"}}},
				)
			}

			if _, err := assetbuild.Build(t.Context(), config, assetbuild.Options{Project: project, Identity: scenario}); err == nil {
				t.Fatal("invalid stage sequence or binding was accepted", scenario)
			}

			if _, err := os.Stat(filepath.Join(project, "generated/public")); !os.IsNotExist(err) {
				t.Fatal("failed processing published partial game output", err)
			}
		})
	}
}

func materialFixtureConfig(staged bool) assetbuild.Config {
	config := fixtureConfig(syntheticBake())
	config.Pipelines[0].Input = append(config.Pipelines[0].Input, "*.png")
	config.Pipelines[0].Import = []assetbuild.Plugin{
		fixtureModels{}, assetbuild.Images(),
		Scenes(SceneOptions{Input: []string{"test.scene.yaml"}, SeparateMasked: true}),
	}

	processing := []assetbuild.Plugin{
		assetbuild.SSBump(assetbuild.SSBumpOptions{Rays: 16, Distance: 2, HeightScale: 3}),
		assetbuild.Mipmaps(),
	}
	if !staged {
		processing = append(processing, assetbuild.KTX2())
	}

	bindings := map[string]map[string]string{"floor.glb": {"floor": "floor-height"}}
	config.Pipelines[0].Process = slices.Concat(
		processing,
		[]assetbuild.Plugin{Materials(MaterialOptions{SSBump: bindings})},
		config.Pipelines[0].Process[1:],
	)

	return config
}

func writeReliefFixture(t *testing.T, project string, slope uint8) {
	t.Helper()

	pixels := image.NewNRGBA(image.Rect(0, 0, 4, 4))

	for y := range 4 {
		for x := range 4 {
			value := uint8(x+y) * slope
			pixels.SetNRGBA(x, y, color.NRGBA{R: value, G: value, B: value, A: 255})
		}
	}

	var output bytes.Buffer
	if err := png.Encode(&output, pixels); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(project, "floor-height.png"), output.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}

func materialOutputFiles(t *testing.T, root string) map[string]string {
	t.Helper()

	files := make(map[string]string)

	err := filepath.WalkDir(root, func(name string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if entry.IsDir() {
			return nil
		}
		//nolint:gosec // G304: filepath.WalkDir visits this test's private output tree.
		data, err := os.ReadFile(name)
		if err != nil {
			return err
		}

		files[strings.TrimPrefix(name, root)] = string(data)

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	return files
}
