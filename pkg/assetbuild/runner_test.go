//go:build !js

package assetbuild

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

var errFixtureFailure = errors.New("fixture failure")

type fixturePlugin func(*Plan) error

func (fixturePlugin) Name() string                 { return "fixture" }
func (plugin fixturePlugin) Plan(plan *Plan) error { return plugin(plan) }

func fixtureConfig(calls *[3]int) Config {
	return Config{Input: []string{"assets/**"}, Output: "generated/assets", Plugins: []Plugin{fixturePlugin(func(plan *Plan) error {
		refs, err := plan.Add(Action{
			ID: "read", Processor: "read", Revision: "1", Outputs: []Slot{{Name: "data", Schema: "text@1"}},
			Run: func(ctx *Context) (Result, error) {
				calls[0]++
				data, err := ctx.ReadFile("assets/source.txt")

				return Result{"data": data}, err
			},
		})
		if err != nil {
			return err
		}

		source := refs[0]

		refs, err = plan.Add(Action{
			ID: "normalize", Processor: "normalize", Revision: "1", Inputs: []Ref{source},
			Outputs: []Slot{{Name: "data", Schema: "text@1"}},
			Run: func(ctx *Context) (Result, error) {
				calls[1]++
				data, err := ctx.Input(source)

				return Result{"data": bytes.TrimSpace(data)}, err
			},
		})
		if err != nil {
			return err
		}

		normalized := refs[0]

		refs, err = plan.Add(Action{
			ID: "emit", Processor: "emit", Revision: "1", Inputs: []Ref{normalized},
			Outputs: []Slot{{Name: "files", Schema: "files@1"}},
			Run: func(ctx *Context) (Result, error) {
				calls[2]++

				data, err := ctx.Input(normalized)
				if err != nil {
					return nil, err
				}

				encoded, err := json.Marshal(Bundle{"value.bin": data})

				return Result{"files": encoded}, err
			},
		})
		if err == nil {
			plan.Publish(refs[0])
		}

		return err
	})}}
}

func TestCacheSkipsExecutionAndStopsAtIdenticalContents(t *testing.T) {
	t.Parallel()
	project := t.TempDir()
	fixtureWrite(t, project, "assets/source.txt", []byte("hello"))

	var calls [3]int

	config := fixtureConfig(&calls)
	options := Options{Project: project, Identity: "fixture-binary@1"}

	report, err := Build(t.Context(), config, options)
	if err != nil || report.Executed != 3 || report.Reused != 0 || calls != [3]int{1, 1, 1} {
		t.Fatalf("cold build: report=%+v calls=%v err=%v", report, calls, err)
	}

	stamp := time.Unix(1000000000, 0)
	for _, name := range []string{"generated/assets/value.bin", "generated/assets/" + manifestName} {
		if err := os.Chtimes(filepath.Join(project, name), stamp, stamp); err != nil {
			t.Fatal(err)
		}
	}

	report, err = Build(t.Context(), config, options)
	if err != nil || report.Executed != 0 || report.Reused != 3 || calls != [3]int{1, 1, 1} {
		t.Fatalf("unchanged build: report=%+v calls=%v err=%v", report, calls, err)
	}

	fixtureWrite(t, project, "assets/source.txt", []byte("hello \n"))

	report, err = Build(t.Context(), config, options)
	if err != nil || report.Executed != 2 || report.Reused != 1 || calls != [3]int{2, 2, 1} {
		t.Fatalf("semantic equality: report=%+v calls=%v err=%v", report, calls, err)
	}

	for _, name := range []string{"generated/assets/value.bin", "generated/assets/" + manifestName} {
		info, err := os.Stat(filepath.Join(project, name))
		if err != nil || !info.ModTime().Equal(stamp) {
			t.Fatalf("unchanged mtime %s: %v %v", name, info, err)
		}
	}

	if err := os.RemoveAll(filepath.Join(project, "generated/assets")); err != nil {
		t.Fatal(err)
	}

	report, err = Build(t.Context(), config, options)
	if err != nil || report.Executed != 0 || report.Written != 1 {
		t.Fatalf("restore: %+v %v", report, err)
	}

	options.NoCache = true

	report, err = Build(t.Context(), config, options)
	if err != nil || report.Executed != 3 || report.Reused != 0 {
		t.Fatalf("bypass: %+v %v", report, err)
	}

	options.NoCache, options.Identity = false, "fixture-binary@2"

	report, err = Build(t.Context(), config, options)
	if err != nil || report.Executed != 3 {
		t.Fatalf("code invalidation: %+v %v", report, err)
	}
}

func TestCorruptCacheRecomputesVerifiedOutputs(t *testing.T) {
	t.Parallel()
	project := t.TempDir()
	fixtureWrite(t, project, "assets/source.txt", []byte("hello"))

	var calls [3]int

	config, options := fixtureConfig(&calls), Options{Project: project, Identity: "fixture"}
	if _, err := Build(t.Context(), config, options); err != nil {
		t.Fatal(err)
	}

	blobs, err := filepath.Glob(filepath.Join(project, "generated/cache/assets/v1/blobs/*"))
	if err != nil || len(blobs) == 0 {
		t.Fatalf("missing cache blobs: %v", err)
	}

	for _, name := range blobs {
		if err := os.WriteFile(name, []byte("broken"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	report, err := Build(t.Context(), config, options)
	if err != nil || report.Executed != 2 || report.Reused != 1 {
		t.Fatalf("corruption: %+v %v", report, err)
	}

	data, err := fixtureRead(project, "generated/assets/value.bin")
	if err != nil || string(data) != "hello" {
		t.Fatalf("output %q: %v", data, err)
	}
}

func TestFailedProcessingDoesNotPublishOrCacheFailure(t *testing.T) {
	t.Parallel()
	project := t.TempDir()
	fixtureWrite(t, project, "assets/source.txt", []byte("hello"))

	var calls [3]int

	config, options := fixtureConfig(&calls), Options{Project: project, Identity: "fixture"}
	if _, err := Build(t.Context(), config, options); err != nil {
		t.Fatal(err)
	}

	wantErr := errFixtureFailure

	config.Plugins = append(config.Plugins, fixturePlugin(func(plan *Plan) error {
		_, err := plan.Add(Action{ID: "fail", Processor: "fail", Revision: "1", Outputs: []Slot{{Name: "data", Schema: "text@1"}},
			Run: func(*Context) (Result, error) { return nil, wantErr }})

		return err
	}))

	fixtureWrite(t, project, "assets/source.txt", []byte("new"))

	if _, err := Build(t.Context(), config, options); !errors.Is(err, wantErr) {
		t.Fatalf("failure: %v", err)
	}

	data, err := fixtureRead(project, "generated/assets/value.bin")
	if err != nil || string(data) != "hello" {
		t.Fatalf("published after failure %q: %v", data, err)
	}

	if _, err := os.Stat(filepath.Join(project, journalName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("journal after failure: %v", err)
	}
}

func TestTrackedDynamicDependencies(t *testing.T) {
	project := t.TempDir()
	fixtureWrite(t, project, "assets/selection.txt", []byte("first.txt"))
	fixtureWrite(t, project, "assets/first.txt", []byte("one"))
	fixtureWrite(t, project, "assets/second.txt", []byte("two"))
	t.Setenv("KARTY_ASSET_TEST_MODE", "one")

	calls := 0
	config := Config{Input: []string{"assets/**"}, Output: "generated/assets", Plugins: []Plugin{fixturePlugin(func(plan *Plan) error {
		refs, err := plan.Add(
			Action{
				ID:        "dynamic",
				Processor: "dynamic",
				Revision:  "1",
				Outputs:   []Slot{{Name: "files", Schema: "files@1"}},
				Run: func(ctx *Context) (Result, error) {
					calls++

					selected, err := ctx.ReadFile("assets/selection.txt")
					if err != nil {
						return nil, err
					}

					data, err := ctx.ReadFile("assets/" + string(selected))
					if err != nil {
						return nil, err
					}

					names, err := ctx.Glob("assets/group/*.txt")
					if err != nil {
						return nil, err
					}

					optional, err := ctx.ReadFile("assets/optional.txt")
					if err != nil && !errors.Is(err, os.ErrNotExist) {
						return nil, err
					}

					value := string(data) + strings.Join(names, ",") + string(optional) + ctx.Getenv("KARTY_ASSET_TEST_MODE")
					encoded, err := json.Marshal(Bundle{"value.bin": []byte(value)})

					return Result{"files": encoded}, err
				},
			},
		)
		if err == nil {
			plan.Publish(refs[0])
		}

		return err
	})}}
	options := Options{Project: project, Identity: "dynamic"}
	build := func(want int) {
		t.Helper()

		if _, err := Build(t.Context(), config, options); err != nil {
			t.Fatal(err)
		}

		if calls != want {
			t.Fatalf("calls %d want %d", calls, want)
		}
	}
	build(1)
	build(1)
	fixtureWrite(t, project, "assets/group/new.txt", []byte("group"))
	build(2)
	fixtureWrite(t, project, "assets/optional.txt", []byte("optional"))
	build(3)
	fixtureWrite(t, project, "assets/selection.txt", []byte("second.txt"))
	build(4)
	fixtureWrite(t, project, "assets/first.txt", []byte("unused"))
	build(4)
	t.Setenv("KARTY_ASSET_TEST_MODE", "two")
	build(5)
}

func TestMultipleOutputSlotsDoNotOverinvalidate(t *testing.T) {
	t.Parallel()
	project := t.TempDir()
	fixtureWrite(t, project, "assets/source.txt", []byte("one"))

	var executed []string

	config := Config{Input: []string{"assets/**"}, Output: "generated/assets", Plugins: []Plugin{fixturePlugin(func(plan *Plan) error {
		refs, err := plan.Add(
			Action{
				ID:        "producer",
				Processor: "producer",
				Revision:  "1",
				Outputs:   []Slot{{Name: "data", Schema: "text@1"}, {Name: "stable", Schema: "text@1"}},
				Run: func(ctx *Context) (Result, error) {
					executed = append(executed, "producer")
					data, err := ctx.ReadFile("assets/source.txt")

					return Result{"data": data, "stable": []byte("unchanged")}, err
				},
			},
		)
		if err != nil {
			return err
		}

		stable := refs[1]

		refs, err = plan.Add(
			Action{
				ID:        "consumer",
				Processor: "consumer",
				Revision:  "1",
				Inputs:    []Ref{stable},
				Outputs:   []Slot{{Name: "files", Schema: "files@1"}},
				Run: func(ctx *Context) (Result, error) {
					executed = append(executed, "consumer")

					data, err := ctx.Input(stable)
					if err != nil {
						return nil, err
					}

					value, err := json.Marshal(Bundle{"value.bin": data})

					return Result{"files": value}, err
				},
			},
		)
		if err == nil {
			plan.Publish(refs[0])
		}

		return err
	})}}

	options := Options{Project: project, Identity: "slots"}
	if _, err := Build(t.Context(), config, options); err != nil {
		t.Fatal(err)
	}

	executed = nil

	fixtureWrite(t, project, "assets/source.txt", []byte("two"))

	if _, err := Build(t.Context(), config, options); err != nil {
		t.Fatal(err)
	}

	if !slices.Equal(executed, []string{"producer"}) {
		t.Fatalf("executed %v", executed)
	}
}

func fixtureWrite(t *testing.T, project, name string, data []byte) {
	t.Helper()

	full := filepath.Join(project, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(full, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func fixtureRead(project, name string) ([]byte, error) {
	return readProjectFile(project, name)
}
