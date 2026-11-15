//go:build !js

package levelbuild

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
	bake "github.com/karty-game/karty-webgpu/pkg/lightmap/bake"
)

// This fixture executes normal scene/UV preparation with the embedded library,
// stopping before GPU transport.
func TestIntegratedProductionPreparationMatchesSeparateArtifacts(t *testing.T) {
	t.Parallel()

	var previous []byte

	for _, integrated := range []bool{false, true} {
		project := writeFixture(t)
		config := fixtureConfig(syntheticBake())

		config.Pipelines[0].Import = append(config.Pipelines[0].Import[:3], preparationCaptureFixture{integrated: integrated})
		if _, err := assetbuild.Build(t.Context(), config, assetbuild.Options{
			Project: project, Identity: "integrated-preparation-equivalence", Jobs: 4, NoCache: true,
		}); err != nil {
			t.Fatal(err)
		}

		//nolint:gosec // The fixed capture name stays inside this test's private generated output.
		captured, err := os.ReadFile(filepath.Join(project, "generated", "preparation.json"))
		if err != nil {
			t.Fatal(err)
		}

		if integrated && !bytes.Equal(previous, captured) {
			t.Fatalf("scene, lighting, receiver ownership or UV artifacts changed:\n%s\n%s", previous, captured)
		}

		previous = captured
	}
}

type preparationCaptureFixture struct{ integrated bool }

func (preparationCaptureFixture) Name() string { return "preparation-capture" }
func (fixture preparationCaptureFixture) Plan(plan *assetbuild.Plan) error {
	plugin := &productionPlugin{
		options: ProductionOptions{MetresPerUnit: 1}, earlyScenes: make(map[string]productionSceneRefs), pageInputs: &productionPageCache{},
	}

	options := UVOptions{Production: true, TexelsPerUnit: 4, MaxPageSize: 32, Padding: 2}
	if fixture.integrated {
		if err := plugin.planIntegratedUVs(plan, options); err != nil {
			return err
		}
	} else if err := LightmapUVs(options).Plan(plan); err != nil {
		return err
	}

	return (stage{name: "capture-preparation", schema: uvSchema, run: func(plan *assetbuild.Plan, resource assetbuild.Resource) error {
		return plan.Expand(assetbuild.Expansion{
			ID: "fixture/preparation", Inputs: []assetbuild.Ref{resource.Ref},
			Run: func(ctx *assetbuild.Context, child *assetbuild.Plan) error {
				return capturePreparedScene(ctx, child, resource, plugin, fixture.integrated)
			},
		})
	}}).Plan(plan)
}

func capturePreparedScene(
	ctx *assetbuild.Context,
	plan *assetbuild.Plan,
	resource assetbuild.Resource,
	plugin *productionPlugin,
	integrated bool,
) error {
	var record partitioned
	if err := readRecord(ctx, resource.Ref, &record); err != nil {
		return err
	}

	chunk := record.Chunks[0]
	key := chunk.Description.Space + "/" + chunk.Occluders.SHA256

	scene, exists := plugin.earlyScenes[resource.ID+"/"+key]
	if !exists {
		var err error

		scene, err = plugin.planScene(plan, resource.ID, key, record, chunk)
		if err != nil {
			return err
		}
	}

	refs, err := plan.Add(assetbuild.Action{
		ID: "fixture/preparation-output", Processor: "fixture-preparation-output", Revision: "1",
		Inputs:  []assetbuild.Ref{resource.Ref, scene.geometry, scene.lighting},
		Outputs: []assetbuild.Slot{{Name: filesSlot, Schema: filesSchema}},
		Run: func(ctx *assetbuild.Context) (assetbuild.Result, error) {
			if integrated {
				if err := checkRetainedReceiverPages(ctx, plugin, record); err != nil {
					return nil, err
				}
			}

			var geometry, lighting productionScene
			if err := readRecord(ctx, scene.geometry, &geometry); err != nil {
				return nil, err
			}

			if err := readRecord(ctx, scene.lighting, &lighting); err != nil {
				return nil, err
			}

			captured, err := encode(struct {
				Geometry, Lighting assetbuild.Artifact
				Chunks             []chunkRecord
			}{geometry.Data, lighting.Data, record.Chunks})
			if err != nil {
				return nil, err
			}

			bundle, err := assetbuild.EncodeBundle(assetbuild.Bundle{"preparation.json": captured})

			return assetbuild.Result{filesSlot: bundle}, err
		},
	})
	if err == nil {
		plan.Publish(refs[0])
	}

	return err
}

// Compare the producer's transferred ownership against the independently admitted
// wire decoder, including nested footprint data and the schema version.
func checkRetainedReceiverPages(ctx *assetbuild.Context, plugin *productionPlugin, record partitioned) error {
	for _, chunk := range record.Chunks {
		for _, page := range chunk.Pages {
			retained, found := plugin.pageInputs.take(page)
			if !found {
				return ErrInvalid
			}

			payload, err := ctx.ReadArtifact(page.Receivers)
			if err != nil {
				return err
			}

			decoded, err := bake.DecodeReceiverSet(payload)
			if err != nil {
				return err
			}

			if !reflect.DeepEqual(retained, decoded) {
				return ErrInvalid
			}
		}
	}

	return nil
}
