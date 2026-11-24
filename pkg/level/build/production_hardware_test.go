//go:build linux

package levelbuild

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/karty-game/ashura/internal/leveldata"
	"github.com/karty-game/ashura/pkg/assetbuild"
	"github.com/karty-game/ashura/pkg/level"
	bake "github.com/karty-game/ashura/pkg/lightmap/bake"
)

//nolint:paralleltest // Hardware worker qualification is intentionally serialized; Exercise the complete asset graph, both representations and their independent admission.
func TestHardwareProductionLevelPublishesBothRepresentations(t *testing.T) {
	for _, fixture := range []struct {
		name   string
		size   [3]float32
		chunks int
	}{
		{"single-cell", [3]float32{32, 32, 32}, 1},
		{"cross-cell", [3]float32{2, 2, 2}, 2},
	} {
		t.Run(fixture.name, func(t *testing.T) { productionHardwareFixture(t, fixture.size, fixture.chunks) })
	}
}

//nolint:gocognit // Hardware fixture owns an isolated asset graph and verifies publication as a complete family.
func productionHardwareFixture(t *testing.T, size [3]float32, expectedChunks int) {
	t.Helper()

	if os.Getenv("KARTY_LIGHTMAPPER_GPU_TEST") != "1" {
		t.Skip("opt-in hardware compute level build")
	}

	project := writeFixture(t)
	// This acceptance fixture uses a low-contrast lamp, within the representable
	// RNM domain; separate directional fixtures exercise rejected high contrast.
	scenePath := filepath.Join(project, "test.scene.yaml")
	if err := os.WriteFile(scenePath, []byte(strings.ReplaceAll(fixtureScene, "intensity: 2", "intensity: 0.1")), 0o600); err != nil {
		t.Fatal(err)
	}

	config := assetbuild.Config{Output: "generated",
		Pipelines: []assetbuild.Pipeline{{Name: "fixture",
			Input: []string{"*.scene.yaml", "*.kmodel"},
			Import: []assetbuild.Plugin{
				fixtureModels{},
				Scenes(SceneOptions{Input: []string{"test.scene.yaml"}}),
				Materials(MaterialOptions{}),
				StaticChunks(ChunkOptions{Size: size, Origin: [3]float32{-1, -2, -8}}),
				LightmapUVs(UVOptions{Production: true, TexelsPerUnit: 4, MaxPageSize: 32, Padding: 2}),
				ProductionBake(
					ProductionOptions{
						Worker:   bake.WorkerOptions{MemoryBytes: 128 << 20},
						Sampling: bake.Options{Outputs: bake.OutputRGB | bake.OutputRNM, Samples: 256, Bounces: 2, Seed: 29},
						Adaptive: &bake.AdaptivePolicy{
							PilotSamples: 32,
							Attempt:      "fixture@1",
							Quality: bake.QualityPolicy{
								Minimum: 256, Maximum: 512, AbsoluteError: .01, RelativeError: .05, Confidence: 2,
							},
						},
						Reconstruction: &bake.ReconstructionOptions{
							Radius:         .4,
							PlaneTolerance: .01,
							NormalCos:      .95,
							TangentCos:     .95,
							NoiseScale:     1,
							Strength:       1,
							MaximumHalo:    2048,
						},
						SurfaceEdges: bake.SurfaceEdgeOptions{MaximumCandidates: 1000000, MaximumEdges: 100000},
						Verification: &bake.VerificationPolicy{
							Samples:               1024,
							AbsoluteTolerance:     .06,
							RelativeTolerance:     .4,
							Confidence:            2,
							RequireAllReceivers:   true,
							RequireMaterialDomain: true,
							MaterialDomains: []bake.MaterialNormalDomain{
								{Material: 0, Identity: "fixture-flat@1", Normals: []bake.Vec3{{0, 0, 1}}},
							},
						},
						Environment:     &bake.Environment{Sky: bake.Vec3{.2, .2, .2}, Ground: bake.Vec3{.2, .2, .2}},
						BackendIdentity: "hardware-fixture-explicit-backend@1",
					},
				),
				PublishVariants(
					PublishOptions{Variant: "RGB", Representation: leveldata.RGBDiffuse},
					PublishOptions{Variant: "RNM", Representation: leveldata.RNMFit},
				),
			},
		}},
	}

	report, err := assetbuild.Build(t.Context(), config, assetbuild.Options{Project: project, Identity: "production-fixture@1"})
	if err != nil {
		t.Fatal(err)
	}

	for _, variant := range []string{"RGB", "RNM"} {
		names, err := filepath.Glob(filepath.Join(project, "generated", "public", "levels", "profiles", variant, "*.json"))
		if err != nil || len(names) != 1 {
			t.Fatal(names, err)
		}

		data, err := os.ReadFile(names[0])
		if err != nil {
			t.Fatal(err)
		}

		doc, err := leveldata.Decode(data)
		if err != nil || doc.Schema != leveldata.Schema6 {
			t.Fatal(doc.Schema, err)
		}

		if len(doc.Chunks) != expectedChunks {
			t.Fatal("receiver jobs changed runtime chunk topology", len(doc.Chunks))
		}

		count := 0

		for _, chunk := range doc.Chunks {
			if _, err = level.ReadChunk(t.Context(), filepath.Join(project, "generated", "public"), chunk); err != nil {
				t.Fatal(err)
			}

			for _, page := range chunk.Pages {
				expected := 3
				if variant == "RGB" {
					expected = 1
				}

				if page.MapCount() != expected {
					t.Fatal(page.Representation, page.MapCount())
				}

				count += len(page.Materials)
				for _, material := range page.Materials {
					if material.DoubleSided {
						t.Fatal("front/back lighting silently shared")
					}
				}
			}
		}

		if count < 2 {
			t.Fatal("independent receiver sides missing", count)
		}
	}

	again, err := assetbuild.Build(t.Context(), config, assetbuild.Options{Project: project, Identity: "production-fixture@1"})
	if err != nil {
		t.Fatal(err)
	}

	if again.Executed != 0 || again.Written != 0 {
		t.Fatalf("unchanged build performed processing: %+v", again)
	}

	if expectedChunks == 1 {
		assertProductionLightingOnlyReuse(t, config, project, scenePath)
	}

	t.Logf(
		"whole pipeline RGB+RNM, independent sides, strict loading, unchanged cache; actions=%d wall=%s",
		report.Executed,
		report.Duration,
	)
}

func assertProductionLightingOnlyReuse(t *testing.T, config assetbuild.Config, project, scenePath string) {
	t.Helper()

	if err := os.WriteFile(scenePath, []byte(strings.ReplaceAll(fixtureScene, "intensity: 2", "intensity: 0.08")), 0o600); err != nil {
		t.Fatal(err)
	}

	changed, buildErr := assetbuild.Build(t.Context(), config, assetbuild.Options{Project: project, Identity: "production-fixture@1"})
	if buildErr != nil {
		t.Fatal(buildErr)
	}

	reusedGeometry, reusedUV, reusedSupport, tracedLighting := false, false, false, false

	for _, action := range changed.Actions {
		switch action.Processor {
		case "lightmapper-geometry":
			if action.Status != "HIT" {
				t.Fatal("lighting edit rebuilt geometry acceleration", action)
			}

			reusedGeometry = true
		case "level-uv":
			if action.Status != "HIT" {
				t.Fatal("lighting edit rebuilt receivers", action)
			}

			reusedUV = true
		case "lightmapper-surface-support":
			if action.Status != "HIT" {
				t.Fatal("lighting edit rebuilt static reconstruction support", action)
			}

			reusedSupport = true
		case "lightmapper-sample":
			tracedLighting = tracedLighting || action.Status == "MISS"
		}
	}

	if !reusedGeometry || !reusedUV || !reusedSupport || !tracedLighting {
		t.Fatal("light-only invalidation did not preserve preparation", changed.Actions)
	}
}
