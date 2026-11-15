//go:build !js

package assetbuild

import (
	"errors"
	"os"
	"slices"
	"testing"
)

func pluginStagesFixture(events *[]string, phase Phase) Config {
	return Config{Output: "generated/assets",
		Import:  []Plugin{stageFixturePlugin("import", events, phase)},
		Process: []Plugin{stageFixturePlugin("process", events, phase)},
		Export:  []Plugin{stageFixturePlugin("export", events, phase)},
	}
}

func stageFixturePlugin(name string, events *[]string, phase Phase) Plugin {
	return fixturePlugin(func(plan *Plan) error {
		*events = append(*events, "plan/"+name)

		if plan.Environment.Phase != phase {
			return errFixtureFailure
		}

		return plan.Expand(Expansion{ID: name + "/discover", Run: func(_ *Context, child *Plan) error {
			*events = append(*events, "expand/"+name)

			var previous Ref
			if len(child.Resources) != 0 {
				previous = child.Resources[len(child.Resources)-1].Ref
			}

			refs, err := child.Add(stageFixtureAction(name, previous, events))
			if err != nil {
				return err
			}

			child.Resources = append(child.Resources, Resource{ID: name, Ref: refs[0]})
			if name == "export" {
				child.Publish(refs[0])
			}

			return nil
		}})
	})
}

func stageFixtureAction(name string, previous Ref, events *[]string) Action {
	action := Action{ID: name, Processor: "stage-fixture", Revision: "1",
		Outputs: []Slot{{Name: "data", Schema: "stage-fixture@1"}},
		Run: func(ctx *Context) (Result, error) {
			*events = append(*events, "run/"+name)

			data := make([]byte, 0, len(name))

			if previous.Action != "" {
				var err error

				data, err = ctx.Input(previous)
				if err != nil {
					return nil, err
				}
			}

			data = append(data, name...)
			if name == "export" {
				encoded, err := EncodeBundle(Bundle{"stages.txt": data})

				return Result{"data": encoded}, err
			}

			return Result{"data": data}, nil
		}}
	if previous.Action != "" {
		action.Inputs = []Ref{previous}
	}

	return action
}

func TestStagedPluginsShareExpansionOrderAndLifecycle(t *testing.T) {
	t.Parallel()

	for _, phase := range []Phase{PhaseGenerate, PhaseBuild} {
		t.Run(string(phase), func(t *testing.T) {
			t.Parallel()

			var events []string

			project := t.TempDir()

			_, err := Build(t.Context(), pluginStagesFixture(&events, phase), Options{Project: project, Identity: "stages@1", Phase: phase})
			if err != nil {
				t.Fatal(err)
			}

			want := []string{"plan/import", "plan/process", "plan/export", "expand/import", "run/import",
				"expand/process", "run/process", "expand/export", "run/export"}
			if !slices.Equal(events, want) {
				t.Fatalf("events: %v; want %v", events, want)
			}

			data, err := fixtureRead(project, "generated/assets/stages.txt")
			if err != nil || string(data) != "importprocessexport" {
				t.Fatalf("shared resources: %q %v", data, err)
			}
		})
	}
}

func TestRecipeAdaptersPreserveCacheIdentitiesAndMatchNamedOutput(t *testing.T) {
	t.Parallel()

	for _, flatFirst := range []bool{false, true} {
		name := "staged-first"
		if flatFirst {
			name = "flat-first"
		}

		t.Run(name, func(t *testing.T) {
			t.Parallel()

			configs := adapterFixtureConfigs(flatFirst)
			options := Options{Project: t.TempDir(), Identity: "stages@1", Profile: "release", TargetOS: "js", TargetArch: "wasm"}

			for index, config := range configs {
				report, err := Build(t.Context(), config, options)
				if err != nil {
					t.Fatal(err)
				}

				assertAdapterFixtureReport(t, report, index)

				data, err := fixtureRead(options.Project, "generated/assets/stages.txt")
				if err != nil || string(data) != "importprocessexport" {
					t.Fatalf("build %d changed prepared output: %q %v", index, data, err)
				}
			}
		})
	}
}

func adapterFixtureConfigs(flatFirst bool) []Config {
	var events []string

	staged := pluginStagesFixture(&events, PhaseBuild)
	staged.Input, staged.Profiles = []string{"assets/**"}, []string{"release"}
	wantEnvironment := Environment{Phase: PhaseBuild, Profile: "release", TargetOS: "js", TargetArch: "wasm"}

	staged.Import = append([]Plugin{fixturePlugin(func(plan *Plan) error {
		if plan.Environment != wantEnvironment || !slices.Equal(plan.Input, staged.Input) {
			return errFixtureFailure
		}

		return nil
	})}, staged.Import...)
	flat := staged
	flat.Plugins = append(slices.Clone(staged.Import), staged.Process[0], staged.Export[0])
	flat.Import, flat.Process, flat.Export = nil, nil, nil

	configs := make([]Config, 0, 4)
	configs = append(configs, staged, flat)

	if flatFirst {
		slices.Reverse(configs)
	}

	named := Config{Output: staged.Output, Profiles: staged.Profiles, Pipelines: []Pipeline{{
		Name: "stages", Input: staged.Input, Import: staged.Import, Process: staged.Process, Output: staged.Export,
	}}}
	// Named recipes isolate their cache. Returning to an adapter must still
	// reuse the historical unprefixed graph after the named build.
	return append(configs, named, configs[0])
}

func assertAdapterFixtureReport(t *testing.T, report Report, index int) {
	t.Helper()

	const actionCount = 3

	fresh := index == 0 || index == 2
	if fresh && report.Executed != actionCount || !fresh && (report.Executed != 0 || report.Reused != actionCount) {
		t.Fatalf("build %d: %+v", index, report)
	}

	want := []string{"import", "process", "export"}
	if index == 2 {
		for action := range want {
			want[action] = "pipeline/stages/" + want[action]
		}
	}

	identities := make([]string, 0, len(report.Actions))
	for _, action := range report.Actions {
		identities = append(identities, action.ID)
	}

	if !slices.Equal(identities, want) {
		t.Fatalf("build %d changed action identities: %v; want %v", index, identities, want)
	}
}

type forbiddenStagePlugin struct{ testing *testing.T }

func (plugin forbiddenStagePlugin) Name() string {
	plugin.testing.Fatal("mixed config invoked plugin Name")

	return "forbidden"
}

func (plugin forbiddenStagePlugin) Plan(*Plan) error {
	plugin.testing.Fatal("mixed config invoked plugin Plan")

	return errFixtureFailure
}

func TestMixedPluginStagesRejectBeforeSideEffects(t *testing.T) {
	t.Parallel()

	for _, stage := range []string{"import", "process", "export"} {
		t.Run(stage, func(t *testing.T) {
			t.Parallel()

			project := t.TempDir()
			plugin := forbiddenStagePlugin{testing: t}
			config := Config{Output: "generated/assets", Plugins: []Plugin{plugin}}

			switch stage {
			case "import":
				config.Import = []Plugin{plugin}
			case "process":
				config.Process = []Plugin{plugin}
			case "export":
				config.Export = []Plugin{plugin}
			}

			_, err := Build(t.Context(), config, Options{Project: project, Identity: "stages@1"})
			if !errors.Is(err, errPlan) {
				t.Fatalf("mixed config error: %v", err)
			}

			entries, err := os.ReadDir(project)
			if err != nil || len(entries) != 0 {
				t.Fatalf("mixed config changed project: %v %v", entries, err)
			}
		})
	}
}
