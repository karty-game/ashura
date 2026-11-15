//go:build !js

package assetbuild

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
	"testing"
)

func pipelineFixtureAction(id string) Action {
	return Action{ID: id, Processor: "pipeline-fixture", Revision: "1",
		Outputs: []Slot{{Name: "data", Schema: "pipeline-fixture@1"}},
		Run:     func(*Context) (Result, error) { return Result{"data": []byte("prepared")}, nil },
	}
}

func TestPipelinePlanNamespacesReferencesAndImageIdentities(t *testing.T) {
	t.Parallel()

	plan := &Plan{namespace: "pipeline/images/"}

	refs, err := plan.Add(pipelineFixtureAction("source"))
	if err != nil {
		t.Fatal(err)
	}

	want := Ref{Action: "pipeline/images/source", Slot: "data"}
	if refs[0] != want || plan.actions[0].namespace != plan.namespace {
		t.Fatalf("scoped action: %+v %+v", refs, plan.actions[0])
	}

	local := Ref{Action: "source", Slot: "data"}
	foreign := Ref{Action: "pipeline/other/source", Slot: "data"}
	action := pipelineFixtureAction("consumer")

	action.Inputs = []Ref{local, want, foreign}
	if _, err := plan.Add(action); err != nil {
		t.Fatal(err)
	}

	wantInputs := []Ref{want, want, {Action: "pipeline/images/pipeline/other/source", Slot: "data"}}
	if !slices.Equal(plan.actions[1].Inputs, wantInputs) {
		t.Fatalf("scoped inputs: %+v", plan.actions[1].Inputs)
	}

	for _, ref := range []Ref{local, want} {
		if schema, err := plan.SlotSchema(ref); err != nil || schema != "pipeline-fixture@1" {
			t.Fatalf("local schema: %q %v", schema, err)
		}
	}

	if _, err := plan.SlotSchema(foreign); !errors.Is(err, errPlan) {
		t.Fatalf("foreign schema admitted: %v", err)
	}

	plan.Publish(local)

	if !slices.Equal(plan.published, []Ref{want}) || imageActionID(plan, "source") != "source#2" {
		t.Fatalf("publication/image identity: %+v %q", plan.published, imageActionID(plan, "source"))
	}

	if _, err := plan.Add(pipelineFixtureAction(want.Action)); !errors.Is(err, errPlan) {
		t.Fatalf("already scoped duplicate admitted: %v", err)
	}
}

func TestPipelinePlansIsolateInputsResourcesAndRetainFailedCleanup(t *testing.T) {
	t.Parallel()

	var events []string

	plugin := fixturePlugin(func(plan *Plan) error {
		if len(plan.Resources) != 0 || len(plan.Input) != 1 {
			return errFixtureFailure
		}

		events = append(events, plan.Input[0])

		refs, err := plan.Add(pipelineFixtureAction("same-local-id"))
		if err != nil {
			return err
		}

		plan.Resources = append(plan.Resources, Resource{ID: "shared-name", Ref: refs[0]})

		return nil
	})
	definitions := []Pipeline{{Name: "first", Input: []string{"first/**"}, Import: []Plugin{plugin}},
		{Name: "second", Input: []string{"second/**"}, Import: []Plugin{plugin}}}

	root := &Plan{Project: "project", Output: "output", Environment: Environment{Phase: PhaseGenerate}}
	if err := planNamedPipelines(root, definitions); err != nil {
		t.Fatal(err)
	}

	if !slices.Equal(events, []string{"first/**", "second/**"}) || len(root.actions) != len(definitions) || len(root.Resources) != 0 {
		t.Fatalf("pipeline isolation: %v %+v", events, root)
	}

	definitions[0].Input[0] = "mutated"
	if root.pipelines[0].Input[0] != "first/**" || root.pipelines[1].Environment != root.Environment {
		t.Fatal("pipeline input/environment ownership changed")
	}

	failed := &Plan{}

	err := planNamedPipelines(failed, []Pipeline{{Name: "failure", Import: []Plugin{fixturePlugin(func(plan *Plan) error {
		plan.cleanups = append(plan.cleanups, func() error { return nil })

		return errFixtureFailure
	})}}})
	if !errors.Is(err, errFixtureFailure) || len(failed.pipelines) != 1 || len(failed.pipelines[0].cleanups) != 1 {
		t.Fatalf("failed pipeline lost cleanup ownership: %+v %v", failed.pipelines, err)
	}
}

func TestPipelineNamesAndCountAdmission(t *testing.T) {
	t.Parallel()

	for _, names := range [][]string{{""}, {"not-valid"}, {"for"}, {"same", "same"}} {
		definitions := make([]Pipeline, 0, len(names))
		for _, name := range names {
			definitions = append(definitions, Pipeline{Name: name})
		}

		if err := validatePluginStages(Config{Pipelines: definitions}); !errors.Is(err, errPlan) {
			t.Fatalf("names %q admitted: %v", names, err)
		}
	}

	if err := validatePipelineNames(make([]Pipeline, maxPlanSteps+1)); !errors.Is(err, errPlan) {
		t.Fatalf("excess pipeline count admitted: %v", err)
	}

	if err := validatePipelineNames([]Pipeline{{Name: "images"}, {Name: "Models_2"}}); err != nil {
		t.Fatalf("valid identifiers rejected: %v", err)
	}
}

func TestPipelineAggregateIdentityBound(t *testing.T) {
	t.Parallel()

	root := &Plan{ids: make(map[string]bool, maxPlanSteps)}
	for index := range maxPlanSteps - 1 {
		root.ids["existing/"+strconv.Itoa(index)] = true
	}

	if err := mergePipelineActions(root, &Plan{ids: map[string]bool{"last": true}}); err != nil {
		t.Fatalf("exact global bound rejected: %v", err)
	}

	if err := mergePipelineActions(root, &Plan{ids: map[string]bool{"excess": true}}); !errors.Is(err, errPlan) {
		t.Fatalf("aggregate graph overflow admitted: %v", err)
	}

	if len(root.ids) != maxPlanSteps || root.ids["excess"] {
		t.Fatal("rejected merge changed admitted graph")
	}
}

func TestPipelineSourceRootConfinementBeforePluginWork(t *testing.T) {
	t.Parallel()

	for _, input := range []string{"../outside/**", "generated/assets/**", "generated/cache/**"} {
		t.Run(fmt.Sprintf("%q", input), func(t *testing.T) {
			t.Parallel()

			project := t.TempDir()

			config := Config{Output: "generated/assets", Pipelines: []Pipeline{{Name: "confined", Input: []string{input},
				Import: []Plugin{forbiddenStagePlugin{testing: t}}}}}
			if _, err := Build(t.Context(), config, Options{Project: project, Identity: "pipeline-roots"}); !errors.Is(err, errPath) {
				t.Fatalf("source root %q admitted: %v", input, err)
			}

			entries, err := os.ReadDir(project)
			if err != nil || len(entries) != 0 {
				t.Fatalf("invalid source root changed project: %v %v", entries, err)
			}
		})
	}
}

func TestPipelineRejectsMixedSharedConfiguration(t *testing.T) {
	t.Parallel()

	configs := []Config{{Input: []string{"assets/**"}}, {Plugins: []Plugin{nil}},
		{Import: []Plugin{nil}}, {Process: []Plugin{nil}}, {Export: []Plugin{nil}}}
	for index, config := range configs {
		t.Run(strconv.Itoa(index), func(t *testing.T) {
			t.Parallel()

			project := t.TempDir()
			config.Output = "generated/assets"
			config.Pipelines = []Pipeline{{Name: "isolated", Import: []Plugin{forbiddenStagePlugin{testing: t}}}}

			if _, err := Build(t.Context(), config, Options{Project: project, Identity: "pipeline-mixed"}); !errors.Is(err, errPlan) {
				t.Fatalf("mixed pipeline configuration admitted: %v", err)
			}

			entries, err := os.ReadDir(project)
			if err != nil || len(entries) != 0 {
				t.Fatalf("mixed pipeline configuration changed project: %v %v", entries, err)
			}
		})
	}
}

func planNamedPipelines(root *Plan, definitions []Pipeline) error {
	pipelines, err := normalizePipelines(Config{Pipelines: definitions})
	if err != nil {
		return err
	}

	return planPipelines(root, pipelines)
}
