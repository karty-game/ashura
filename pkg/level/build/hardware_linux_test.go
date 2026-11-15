//go:build linux

package levelbuild

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/karty-game/karty-webgpu/internal/ktx2data"
	"github.com/karty-game/karty-webgpu/internal/leveldata"
	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
	bake "github.com/karty-game/karty-webgpu/pkg/lightmap/bake"
)

//nolint:paralleltest // One native worker qualifies cross-chunk visibility without concurrent GPU fixtures.
func TestHardwareLevelBakeSeesOccluderInAnotherChunk(t *testing.T) {
	if os.Getenv("KARTY_LIGHTMAPPER_GPU_TEST") != "1" && os.Getenv("KARTY_LIGHTMAP_GPU_TEST") != "1" {
		t.Skip("opt-in native hardware compute")
	}

	project := writeFixture(t)

	scene := strings.Replace(fixtureScene, "    - id: power", `    - id: ceiling
      components:
        spatial.transform3d: {position: [0, 1, 0]}
        render3d.model: {asset: floor.glb}
    - id: power`, 1)
	if err := os.WriteFile(filepath.Join(project, "test.scene.yaml"), []byte(scene), 0o600); err != nil {
		t.Fatal(err)
	}

	config := assetbuild.Config{Output: "generated",
		Pipelines: []assetbuild.Pipeline{{Name: "fixture",
			Input: []string{"*.scene.yaml", "*.kmodel"},
			Import: []assetbuild.Plugin{
				fixtureModels{},
				ImportScenes(ImportOptions{Input: []string{"test.scene.yaml"}}),
				StaticChunks(ChunkOptions{Size: [3]float32{1, 1, 1}}),
				LightmapUVs(UVOptions{Production: true, TexelsPerUnit: 4, MaxPageSize: 32, Padding: 2}),
				ProductionBake(ProductionOptions{
					Worker: bake.WorkerOptions{MemoryBytes: 128 << 20},
					// Zero bounces isolates the original direct-visibility assertion:
					// the ceiling blocks every floor receiver across a chunk boundary.
					Sampling:        bake.Options{Outputs: bake.OutputRNM, Samples: 16, Seed: 29},
					Preview:         true,
					BackendIdentity: "native-cross-chunk-visibility-fixture@1",
				}),
				Publish(PublishOptions{}),
			},
		}},
	}

	report, err := assetbuild.Build(t.Context(), config, assetbuild.Options{Project: project, Identity: "hardware-level-fixture@1"})
	if err != nil {
		t.Fatal(err)
	}

	doc := findManifest(t, project)
	dark, lit := 0, 0

	for _, chunk := range doc.Chunks {
		for _, page := range chunk.Pages {
			peak := rnmPagePeak(t, project, page)

			if chunk.Sources[0] == "/floor" {
				dark++

				if peak != 0 {
					t.Fatalf("outside-chunk ceiling failed to block floor: peak=%d chunk=%s", peak, chunk.ID)
				}
			}

			if chunk.Sources[0] == "/ceiling" {
				lit++

				if peak == 0 {
					t.Fatal("unblocked ceiling received no pointlight")
				}
			}
		}
	}

	if dark != 4 || lit != 4 {
		t.Fatalf("missing receiver pages floor=%d ceiling=%d", dark, lit)
	}

	t.Logf(
		"eight chunk pages: four shadowed floor/four lit ceilings, shared GPU worker, actions=%d wall=%s",
		report.Executed,
		report.Duration,
	)
}

func rnmPagePeak(t *testing.T, project string, page leveldata.Page) byte {
	t.Helper()

	peak := byte(0)

	for _, ref := range page.Maps {
		data := readLevelFixture(t, project, filepath.Join("generated/public", ref.Path))

		image, err := ktx2data.Decode(data)
		if err != nil {
			t.Fatal(err)
		}

		for offset := 0; offset < len(image.Levels[0]); offset += 4 {
			peak = max(peak, image.Levels[0][offset], image.Levels[0][offset+1], image.Levels[0][offset+2])
		}
	}

	return peak
}
