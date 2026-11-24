//go:build !js

package levelbuild

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/karty-game/ashura/internal/leveldata"
	"github.com/karty-game/ashura/internal/lightmapper"
	"github.com/karty-game/ashura/pkg/assetbuild"
	bake "github.com/karty-game/ashura/pkg/lightmap/bake"
)

func productionSamplePagesFixture() []pageRecord {
	return []pageRecord{
		{Receivers: assetbuild.Artifact{SHA256: strings.Repeat("a", 64), Size: 1}, Geometry: codecFixtureFile().Artifact},
		{Receivers: assetbuild.Artifact{SHA256: strings.Repeat("b", 64), Size: 2}, Geometry: codecFixtureFile().Artifact},
	}
}

func TestProductionSpaceSamplesBatchCanonicalPagesAndRetainFallbacks(t *testing.T) {
	t.Parallel()

	pages := productionSamplePagesFixture()
	group := &productionSpacePlan{
		scene: assetbuild.Ref{Action: "scene", Slot: slot},
		chunks: []chunkRecord{
			{Description: leveldata.Chunk{ID: "first"}, Pages: pages[:1]},
			{Description: leveldata.Chunk{ID: "second"}, Pages: pages[1:]},
		},
	}
	combined := chunkRecord{Description: leveldata.Chunk{ID: "space/room"}, Pages: pages}

	for _, mode := range []string{"fixed", "adaptive", "resume", "explicit-resume"} {
		plugin := productionPlugin{}

		switch mode {
		case "adaptive":
			plugin.options.Adaptive = &bake.AdaptivePolicy{}
		case "resume":
			plugin.resume = []assetbuild.Ref{{Action: "resume", Slot: slot}}
		case "explicit-resume":
			plugin.options.ResumeSamples = []string{"*.linear"}
		}

		var plan assetbuild.Plan

		refs, err := plugin.planSpaceSamples(&plan, "fixture", group, combined)
		if err != nil || len(refs) != len(pages) {
			t.Fatal("page roster changed", mode, refs, err)
		}

		if mode == "fixed" {
			if refs[0].Action != refs[1].Action || refs[0].Slot != "page-0" || refs[1].Slot != "page-1" {
				t.Fatal("fixed pages did not share one canonical sampling action", refs)
			}
		} else if refs[0].Action == refs[1].Action {
			t.Fatal("adaptive or resume mode lost per-page actions", mode, refs)
		}

		for _, ref := range refs {
			if schema, err := plan.SlotSchema(ref); err != nil || schema != "karty.lightmapper-sampled-page@1" {
				t.Fatal("sample references changed schema", ref, schema, err)
			}
		}
	}
}

func TestProductionSampleBatchIdentityIncludesOrderedRosterAndBackendPolicy(t *testing.T) {
	t.Parallel()

	pages := productionSamplePagesFixture()
	plugin := productionPlugin{workerResource: assetbuild.NewExecutionResource()}
	scene := assetbuild.Ref{Action: "scene", Slot: slot}

	action := plugin.sampleBatchAction("fixture", "room", pages, scene)
	if !action.Parallel || action.Exclusive != plugin.workerResource || action.Cache != assetbuild.CacheDisabled ||
		action.Processor != "lightmapper-sample" || len(action.Inputs) != 1 || action.Inputs[0] != scene ||
		len(action.Artifacts) != len(pages)*2 {
		t.Fatal("batch lost scene/artifact dependencies, worker exclusion or cache policy", action)
	}

	options, err := json.Marshal(action.Options)
	if err != nil {
		t.Fatal(err)
	}

	reordered, err := json.Marshal(plugin.sampleBatchAction("fixture", "room", []pageRecord{pages[1], pages[0]}, scene).Options)
	if err != nil || bytes.Equal(options, reordered) {
		t.Fatal("canonical artifact sets aliased a reordered page roster", err)
	}

	plugin.options.BackendIdentity = "fixture-device@1"
	if action := plugin.sampleBatchAction("fixture", "room", pages, scene); action.Cache != assetbuild.CacheAutomatic {
		t.Fatal("explicit backend identity did not enable ordinary action caching")
	}
}

func TestProductionGPUActionsDependOnDetectedBackend(t *testing.T) {
	t.Parallel()

	var plan assetbuild.Plan

	plugin := productionPlugin{}
	if err := plugin.planBackend(&plan, "fixture"); err != nil {
		t.Fatal(err)
	}

	ref := plugin.backend
	if schema, err := plan.SlotSchema(ref); err != nil || schema != "karty.lightmapper-backend@1" {
		t.Fatalf("missing backend output: %q %v", schema, err)
	}

	scene := assetbuild.Ref{Action: "scene", Slot: slot}
	pages := productionSamplePagesFixture()
	sample := plugin.sampleBatchAction("fixture", "room", pages, scene)
	resolve := plugin.chunkResolveAction("fixture", chunkRecord{Pages: pages}, scene, nil, assetbuild.Ref{})

	for _, action := range []assetbuild.Action{sample, resolve} {
		if action.Cache != assetbuild.CacheAutomatic || !slices.Contains(action.Inputs, ref) {
			t.Fatalf("%s does not use the detected backend for cache lookup", action.Processor)
		}
	}
}

func productionSampleLinearFixture(identity uint64) bake.Result {
	return bake.Result{
		Schema: lightmapper.ResultSchema, Algorithm: lightmapper.Algorithm,
		SceneHash: strings.Repeat("a", 64), ReceiverHash: fmt.Sprintf("%064x", identity), Backend: "fixture-device",
		Options:   bake.Options{Outputs: bake.OutputRGB, Samples: 2},
		Estimates: []bake.Estimate{{ReceiverID: identity, Count: 2, VarianceValid: true}},
	}
}

func TestProductionSampleBatchResultsKeepPageOrderAndNeverReturnPartialReferences(t *testing.T) {
	t.Parallel()

	linear := []bake.Result{productionSampleLinearFixture(71), productionSampleLinearFixture(29)}
	artifacts := make(map[assetbuild.Artifact][]byte)
	store := func(data []byte) (assetbuild.Artifact, error) {
		artifact := assetbuild.Artifact{SHA256: fmt.Sprintf("%x", sha256.Sum256(data)), Size: len(data)}
		artifacts[artifact] = data

		return artifact, nil
	}

	outputs, err := productionSampleBatchResult(linear, store)
	if err != nil || len(outputs) != len(linear) {
		t.Fatal(outputs, err)
	}

	for index, expected := range linear {
		var record struct {
			Linear  assetbuild.Artifact `json:"linear"`
			Resumed bool                `json:"resumed"`
		}
		if err := json.Unmarshal(outputs[productionSampleSlot(index)], &record); err != nil {
			t.Fatal(err)
		}

		decoded, err := bake.DecodeResult(artifacts[record.Linear])
		if err != nil || record.Resumed || decoded.Estimates[0].ReceiverID != expected.Estimates[0].ReceiverID {
			t.Fatal("page output reference was reordered", index, decoded, err)
		}
	}

	linear[1].Estimates = nil
	if partial, err := productionSampleBatchResult(linear, store); err == nil || partial != nil {
		t.Fatal("failed later page exposed a partial output map", partial, err)
	}
}

type productionBatchCacheFixture struct {
	linear []bake.Result
	runs   int
}

func (*productionBatchCacheFixture) Name() string { return "production-batch-cache-fixture" }

func (fixture *productionBatchCacheFixture) Plan(plan *assetbuild.Plan) error {
	plugin := productionPlugin{options: ProductionOptions{
		BackendIdentity: "fixture-device@1", Sampling: fixture.linear[0].Options,
	}}
	action := plugin.sampleBatchAction("fixture", "room", productionSamplePagesFixture(), assetbuild.Ref{})
	// Replace only the worker completion seam: this exercises the real batch
	// output transaction and cache policy without a graphics device fixture.
	action.Inputs, action.Artifacts = nil, nil
	action.Run = func(ctx *assetbuild.Context) (assetbuild.Result, error) {
		fixture.runs++

		return productionSampleBatchResult(fixture.linear, ctx.StoreArtifact)
	}

	_, err := plan.Add(action)

	return err
}

func TestProductionSampleBatchFailureCannotPopulateCache(t *testing.T) {
	t.Parallel()

	fixture := &productionBatchCacheFixture{linear: []bake.Result{productionSampleLinearFixture(71), {}}}
	config := assetbuild.Config{
		Output:    "generated",
		Pipelines: []assetbuild.Pipeline{{Name: "fixture", Import: []assetbuild.Plugin{fixture}}},
	}

	options := assetbuild.Options{Project: t.TempDir(), Identity: "batch-output-transaction@1"}
	if _, err := assetbuild.Build(t.Context(), config, options); err == nil || fixture.runs != 1 {
		t.Fatal("invalid complete output unexpectedly succeeded", fixture.runs, err)
	}

	fixture.linear[1] = productionSampleLinearFixture(29)
	if report, err := assetbuild.Build(t.Context(), config, options); err != nil || report.Executed != 1 || fixture.runs != 2 {
		t.Fatal("failed batch acquired a usable cache record", report, fixture.runs, err)
	}

	if report, err := assetbuild.Build(t.Context(), config, options); err != nil || report.Reused != 1 || fixture.runs != 2 {
		t.Fatal("complete batch did not cache atomically", report, fixture.runs, err)
	}
}
