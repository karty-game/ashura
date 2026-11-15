//go:build !js

package levelbuild

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/karty-game/karty-webgpu/internal/lightmapper"
	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
	bake "github.com/karty-game/karty-webgpu/pkg/lightmap/bake"
)

func TestProductionResumeSelectsExactProvenanceAndRejectsAmbiguity(t *testing.T) {
	t.Parallel()

	options := bake.Options{Outputs: bake.OutputRGB | bake.OutputRNM, Samples: 128, Bounces: 4, Seed: 19}

	compatible := productionResume{
		SceneHash: strings.Repeat("a", 64), ReceiverHash: strings.Repeat("b", 64), Options: options, Backend: "hardware fixture",
		Linear: assetbuild.Artifact{SHA256: strings.Repeat("c", 64), Size: 512},
	}
	for _, mutate := range []func(*productionResume){
		func(record *productionResume) { record.SceneHash = strings.Repeat("d", 64) },
		func(record *productionResume) { record.ReceiverHash = strings.Repeat("d", 64) },
		func(record *productionResume) { record.Options.Seed++ },
		func(record *productionResume) { record.Options.Samples++ },
		func(record *productionResume) { record.Options.Outputs = bake.OutputRGB },
		func(record *productionResume) { record.Options.Bounces++ },
		func(record *productionResume) { record.Options.RouletteDepth++ },
		func(record *productionResume) { record.Options.ExactLights = true },
		func(record *productionResume) { record.Backend = "different adapter" },
	} {
		record := compatible
		mutate(&record)

		_, found, err := selectResumeArtifact(
			[]productionResume{record}, compatible.SceneHash, compatible.ReceiverHash, options, compatible.Backend,
		)
		if err != nil || found {
			t.Fatal("incompatible provenance resumed", record, err)
		}
	}

	selected, found, err := selectResumeArtifact(
		[]productionResume{compatible, compatible}, compatible.SceneHash, compatible.ReceiverHash, options, compatible.Backend,
	)
	if err != nil || !found || selected != compatible.Linear {
		t.Fatal("identical explicitly supplied result should be unambiguous", selected, found, err)
	}

	other := compatible

	other.Linear.SHA256 = strings.Repeat("d", 64)
	if _, _, err := selectResumeArtifact(
		[]productionResume{compatible, other}, compatible.SceneHash, compatible.ReceiverHash, options, compatible.Backend,
	); !errors.Is(err, ErrInvalid) {
		t.Fatal("distinct compatible results silently selected", err)
	}
}

func TestProductionResumeAdmitsCompleteFixedOrderedStatistics(t *testing.T) {
	t.Parallel()

	options := bake.Options{Outputs: bake.OutputRGB, Samples: 4, Bounces: 4}
	receivers := []bake.Receiver{{ID: 71}, {ID: 92}}

	fixture := bake.Result{
		SceneHash: strings.Repeat("a", 64), ReceiverHash: strings.Repeat("b", 64), Options: options, Backend: "hardware fixture",
		Estimates: []bake.Estimate{
			{ReceiverID: 71, Count: 4, VarianceValid: true}, {ReceiverID: 92, Count: 4, VarianceValid: true},
		},
	}
	if err := validateResumeResult(fixture, fixture.SceneHash, fixture.ReceiverHash, options, fixture.Backend, receivers); err != nil {
		t.Fatal("valid fixed production samples rejected", err)
	}

	for _, mutate := range []func(*bake.Result){
		func(result *bake.Result) {
			result.Estimates[0], result.Estimates[1] = result.Estimates[1], result.Estimates[0]
		},
		func(result *bake.Result) { result.Estimates[0].Count = 3 },
		func(result *bake.Result) { result.Estimates[0].VarianceValid = false },
		func(result *bake.Result) { result.Estimates = result.Estimates[:1] },
		func(result *bake.Result) { result.Backend = "different adapter" },
		func(result *bake.Result) { result.Options.Seed++ },
	} {
		changed := fixture
		changed.Estimates = append([]bake.Estimate(nil), fixture.Estimates...)
		mutate(&changed)

		if err := validateResumeResult(changed, fixture.SceneHash, fixture.ReceiverHash, options, fixture.Backend, receivers); err == nil {
			t.Fatal("incomplete or incompatible samples admitted", changed)
		}
	}

	fixture.Options.Samples = 1
	for index := range fixture.Estimates {
		fixture.Estimates[index].Count = 1
		fixture.Estimates[index].VarianceValid = false
	}

	if err := validateResumeResult(
		fixture,
		fixture.SceneHash,
		fixture.ReceiverHash,
		fixture.Options,
		fixture.Backend,
		receivers,
	); err != nil {
		t.Fatal("one fixed sample has no defined sample variance", err)
	}
}

func TestProductionResumePlanningDeduplicatesAndSortsExplicitSources(t *testing.T) {
	t.Parallel()

	project := t.TempDir()
	for _, name := range []string{"b.linear", "a.linear"} {
		if err := os.WriteFile(filepath.Join(project, name), []byte("not read during planning"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	plugin := productionPlugin{options: ProductionOptions{ResumeSamples: []string{"*.linear", "a.linear"}}}
	if err := plugin.planResumeInputs(&assetbuild.Plan{Project: project}); err != nil {
		t.Fatal(err)
	}

	if len(plugin.resume) != 2 || !strings.HasSuffix(plugin.resume[0].Action, "/a.linear") ||
		!strings.HasSuffix(plugin.resume[1].Action, "/b.linear") {
		t.Fatal("explicit glob discovery is not unique and sorted", plugin.resume)
	}

	plugin.options.Adaptive = &bake.AdaptivePolicy{}
	if err := plugin.planResumeInputs(&assetbuild.Plan{Project: project}); !errors.Is(err, ErrInvalid) || len(plugin.resume) != 0 {
		t.Fatal("adaptive resume needs frozen allocation metadata", err, plugin.resume)
	}
}

func TestProductionResumeImportsDeclaredSourceAndRejectsBadChecksum(t *testing.T) {
	t.Parallel()

	fixture := bake.Result{
		Schema: lightmapper.ResultSchema, Algorithm: lightmapper.Algorithm,
		SceneHash: strings.Repeat("a", 64), ReceiverHash: strings.Repeat("b", 64), Backend: "hardware fixture",
		Options:   bake.Options{Outputs: bake.OutputRGB, Samples: 2},
		Estimates: []bake.Estimate{{ReceiverID: 71, Count: 2, VarianceValid: true}},
	}

	data, err := bake.EncodeResult(fixture)
	if err != nil {
		t.Fatal(err)
	}

	project := t.TempDir()

	path := filepath.Join(project, "saved.linear")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	plugin := &resumeSourceFixture{expected: data}

	config := assetbuild.Config{Output: "generated",
		Pipelines: []assetbuild.Pipeline{{Name: "fixture",
			Input:  []string{"*.linear"},
			Import: []assetbuild.Plugin{plugin},
		}},
	}
	if _, err := assetbuild.Build(
		t.Context(),
		config,
		assetbuild.Options{Project: project, Identity: "resume-source-fixture"},
	); err != nil {
		t.Fatal("declared source was not imported with artifact provenance", err)
	}

	corrupt := append([]byte(nil), data...)

	corrupt[len(corrupt)-1] ^= 1
	if err := os.WriteFile(path, corrupt, 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := assetbuild.Build(
		t.Context(),
		config,
		assetbuild.Options{Project: project, Identity: "resume-source-fixture"},
	); err == nil {
		t.Fatal("changed source bypassed typed result checksum admission")
	}
}

type resumeSourceFixture struct{ expected []byte }

func (*resumeSourceFixture) Name() string { return "resume-source-fixture" }

func (fixture *resumeSourceFixture) Plan(plan *assetbuild.Plan) error {
	plugin := productionPlugin{options: ProductionOptions{ResumeSamples: []string{"*.linear"}}}
	if err := plugin.planResumeInputs(plan); err != nil {
		return err
	}

	_, err := plan.Add(assetbuild.Action{
		ID: "resume-consumer", Processor: "resume-fixture", Revision: "1", Inputs: plugin.resume,
		Outputs: []assetbuild.Slot{{Name: slot, Schema: "fixture@1"}},
		Run: func(ctx *assetbuild.Context) (assetbuild.Result, error) {
			var record productionResume
			if err := readRecord(ctx, plugin.resume[0], &record); err != nil {
				return nil, err
			}

			data, err := ctx.ReadArtifact(record.Linear)
			if err != nil {
				return nil, err
			}

			if !bytes.Equal(data, fixture.expected) || record.Options.Samples != 2 || record.Backend != "hardware fixture" {
				return nil, fmt.Errorf("import changed the original result payload or metadata: %w", ErrInvalid)
			}

			return assetbuild.Result{slot: nil}, nil
		},
	})

	return err
}
