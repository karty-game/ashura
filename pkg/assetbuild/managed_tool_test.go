//go:build !js

package assetbuild

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func managedMiseTestContext(t *testing.T, ctx context.Context) *Context {
	t.Helper()

	run, err := newRunner(Config{Output: "generated"}, Options{Project: t.TempDir(), Identity: "mise-adapter"})
	if err != nil {
		t.Fatal(err)
	}

	return &Context{Context: ctx, runner: run}
}

//nolint:paralleltest // The fake mise fixture changes process-wide environment.
func TestManagedToolMiseInvocationAndPerBuildSnapshot(t *testing.T) {
	fixture := newManagedMiseFixture(t)
	ctx := managedMiseTestContext(t, t.Context())

	for range 2 {
		installed, err := ctx.ManagedTool(fixture.spec)
		if err != nil || installed.Executable != filepath.Join(fixture.root, filepath.FromSlash(fixture.spec.Executable)) ||
			!slices.Equal(installed.Components, []string{filepath.Join(fixture.root, "lib/component.so")}) {
			t.Fatal("mise installation did not resolve tracked files", installed, err)
		}
	}

	calls := fixture.calls(t)
	if len(calls) != 2 || calls[0][3] != "install" || calls[1][3] != "where" || calls[0][4] != calls[1][4] {
		t.Fatal("installation must run once, then query the identical pinned tool", calls)
	}

	argument := calls[0][4]
	for _, part := range []string{"http:karty-asset-fixture-", "url=" + fixture.spec.URL,
		"checksum=sha256:" + fixture.spec.SHA256, "format=zip", "strip_components=0", "bin_path=bin", "]@1.0.0"} {
		if !strings.Contains(argument, part) {
			t.Fatal("mise invocation lost pinned artifact options", part, argument)
		}
	}

	changed := fixture.spec

	changed.SHA256 = strings.Repeat("c", 64)
	if _, err := ctx.ManagedTool(changed); err != nil {
		t.Fatal(err)
	}

	calls = fixture.calls(t)
	if len(calls) != 4 || strings.Split(calls[0][4], "[")[0] == strings.Split(calls[2][4], "[")[0] {
		t.Fatal("changed pins reused an existing mise tool identity", calls)
	}
}

func managedMiseFixtureConfig(spec ManagedToolSpec, calls *int) Config {
	return Config{Output: "generated", Pipelines: []Pipeline{{Name: "tool", Import: []Plugin{fixturePlugin(func(plan *Plan) error {
		_, err := plan.Add(Action{ID: "prepare", Processor: "mise-fixture", Revision: "1", Options: spec,
			Outputs: []Slot{{Name: "prepared", Schema: "prepared@1"}}, Run: func(ctx *Context) (Result, error) {
				(*calls)++

				installed, err := ctx.ManagedTool(spec)
				if err != nil {
					return nil, err
				}

				return Result{"prepared": []byte(installed.Executable)}, nil
			}})

		return err
	})}}}}
}

//nolint:paralleltest // The fake mise fixture changes process-wide environment.
func TestManagedToolTracksMiseComponentsExecutableAndEnvironment(t *testing.T) {
	fixture := newManagedMiseFixture(t)
	calls := 0
	config := managedMiseFixtureConfig(fixture.spec, &calls)
	options := Options{Project: t.TempDir(), Identity: "mise-dependencies"}

	for _, executed := range []int{1, 0} {
		if report, err := Build(t.Context(), config, options); err != nil || report.Executed != executed {
			t.Fatal("unchanged tool dependencies did not reuse their action", report, err)
		}
	}

	changes := []func(){
		func() { fixture.write(t, filepath.Join(fixture.root, "lib/component.so"), []byte("changed component")) },
		func() {
			fixture.write(t, filepath.Join(fixture.root, filepath.FromSlash(fixture.spec.Executable)), []byte("changed executable"))
		},
		func() {
			fixture.write(t, fixture.mise, append(slices.Clone(fixture.binary), []byte("changed mise")...))
		},
		func() { t.Setenv("MISE_DATA_DIR", t.TempDir()) },
	}
	for _, change := range changes {
		change()

		if report, err := Build(t.Context(), config, options); err != nil || report.Executed != 1 {
			t.Fatal("changed tool dependency reused a stale action", report, err)
		}
	}

	if calls != 1+len(changes) || len(fixture.calls(t)) != 2*calls {
		t.Fatal("tool identity or once-per-build resolution changed", calls, fixture.calls(t))
	}
}

func TestManagedToolMissingMiseHasNoInstallerFallback(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	ctx := managedMiseTestContext(t, t.Context())
	spec := ManagedToolSpec{Name: "fixture", Version: "1", URL: "https://tools.invalid/tool.zip",
		SHA256: strings.Repeat("a", 64), Format: managedZip, Executable: "bin/tool"}

	if _, err := ctx.ManagedTool(spec); err == nil || !strings.Contains(err.Error(), "requires mise >=2026.11.5") {
		t.Fatal("missing mise did not identify the required developer tool", err)
	}
}

//nolint:paralleltest // The fake mise fixture changes process-wide environment.
func TestManagedToolMiseFailuresAreBoundedAndRejectInvalidWhere(t *testing.T) {
	fixture := newManagedMiseFixture(t)

	for _, mode := range []string{"fail", "relative"} {
		t.Setenv("KARTY_MISE_MODE", mode)
		ctx := managedMiseTestContext(t, t.Context())

		_, err := ctx.ManagedTool(fixture.spec)
		if err == nil || len(err.Error()) > 2*managedMiseLogLimit+1024 {
			t.Fatal("mise failure was admitted or diagnostics were unbounded", mode, err)
		}
	}
}

func TestManagedToolMiseCancellation(t *testing.T) {
	fixture := newManagedMiseFixture(t)
	t.Setenv("KARTY_MISE_MODE", "wait")

	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()

	if _, err := managedMiseTestContext(t, ctx).ManagedTool(fixture.spec); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("mise subprocess did not propagate caller cancellation", err)
	}
}

func TestManagedToolRejectsInvalidSpecsBeforeMiseOrExpansionIO(t *testing.T) {
	t.Parallel()

	base := ManagedToolSpec{Name: "fixture", Version: "1", URL: "https://tools.invalid/tool.zip",
		SHA256: strings.Repeat("a", 64), Format: managedZip, Executable: "bin/tool"}
	for _, change := range []func(*ManagedToolSpec){
		func(spec *ManagedToolSpec) { spec.Name = "http:injected" },
		func(spec *ManagedToolSpec) { spec.Version = "1[hook=x]" },
		func(spec *ManagedToolSpec) { spec.URL = "http://tools.invalid/tool.zip" },
		func(spec *ManagedToolSpec) { spec.URL = "https://user:pass@tools.invalid/tool.zip" },
		func(spec *ManagedToolSpec) { spec.URL = "https://tools.invalid/tool.zip,hook=bad" },
		func(spec *ManagedToolSpec) { spec.SHA256 = "invalid" },
		func(spec *ManagedToolSpec) { spec.Executable = "../tool" },
		func(spec *ManagedToolSpec) { spec.Components = []string{"bin/tool"} },
		func(spec *ManagedToolSpec) { spec.Components = []string{"lib/../tool"} },
		func(spec *ManagedToolSpec) { spec.StripComponents = -1 },
	} {
		spec := base
		change(&spec)

		ctx := &Context{Context: t.Context()}
		if _, err := ctx.ManagedTool(spec); !errors.Is(err, errManagedTool) {
			t.Fatal("invalid artifact reached external tool lookup", spec, err)
		}
	}

	ctx := &Context{Context: t.Context(), expanding: true}
	if _, err := ctx.ManagedTool(base); !errors.Is(err, errPlan) {
		t.Fatal("manifest discovery attempted tool installation", err)
	}
}

func TestManagedMiseOutputRetainsBoundedDiagnostics(t *testing.T) {
	t.Parallel()

	output := &managedMiseOutput{}

	input := []byte(strings.Repeat("x", 2*managedMiseLogLimit))
	if count, err := output.Write(input); err != nil || count != len(input) || len(output.String()) != managedMiseLogLimit {
		t.Fatal("mise output was not drained with a bounded retained prefix", count, err)
	}
}
