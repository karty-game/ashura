//go:build !js

package assetbuild

import (
	"strconv"
	"testing"
)

func TestRecipeRejectsIncompatibleInvocation(t *testing.T) {
	t.Parallel()

	called := false

	for _, args := range [][]string{
		nil,
		{"--protocol=99", "--project=."},
		{"--protocol=2", "--project=."},
		{"--protocol=2", "--project=.", "--phase=unknown"},
		{"--protocol=1", "--project=.", "--phase=generate"},
		{"--protocol=1", "--project=.", "--target=wasm"},
		{"--protocol=1", "--project=.", "--profile="},
		{"--protocol=1", "--project=.", "extra"},
		{"--protocol=1", "--project=.", "--jobs=-1"},
		{"--protocol=1", "--project=.", "--jobs=65537"},
		{"--protocol=1", "--project=.", "--jobs=invalid"},
	} {
		_, err := RunRecipe(t.Context(), args, func(Environment) Config {
			called = true

			return Config{}
		})
		if err == nil {
			t.Errorf("accepted incompatible invocation %v", args)
		}
	}

	if called {
		t.Fatal("evaluated recipe before validating invocation")
	}
}

func TestRecipeLifecyclePhasesShareCachedOutputs(t *testing.T) {
	t.Parallel()
	project := t.TempDir()
	fixtureWrite(t, project, "assets/source.txt", []byte("prepared"))

	var calls [3]int

	for _, phase := range []Phase{PhaseGenerate, PhaseBuild} {
		report, err := RunRecipe(t.Context(), []string{
			"--protocol=2", "--project=" + project, "--phase=" + string(phase), "--target=js/wasm",
		}, func(environment Environment) Config {
			if environment.Phase != phase || environment.TargetOS != "js" || environment.TargetArch != "wasm" {
				t.Fatalf("recipe environment: %+v", environment)
			}

			config := fixtureConfig(&calls)
			config.Plugins = append([]Plugin{fixturePlugin(func(plan *Plan) error {
				if plan.Environment != environment {
					t.Fatalf("plugin environment: %+v; recipe: %+v", plan.Environment, environment)
				}

				return nil
			})}, config.Plugins...)

			return config
		})
		if err != nil {
			t.Fatal(err)
		}

		if phase == PhaseGenerate && report.Executed != 3 || phase == PhaseBuild && report.Reused != 3 {
			t.Fatalf("phase %s: %+v", phase, report)
		}
	}
}

func TestRecipeJobsAdmission(t *testing.T) {
	t.Parallel()

	for _, jobs := range []int{0, 1, 2, maxPlanSteps} {
		invocation, err := recipeOptions([]string{"--protocol=1", "--project=.", "--jobs=" + strconv.Itoa(jobs)})

		options := invocation.options
		if err != nil || options.Jobs != jobs || options.Phase != PhaseBuild {
			t.Fatal("recipe jobs changed", jobs, options.Jobs, err)
		}
	}
}
