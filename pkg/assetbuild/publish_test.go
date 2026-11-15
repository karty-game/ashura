//go:build !js

package assetbuild

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

var errStopAfterRecovery = errors.New("stop after recovery")

func TestOutputOwnershipAndPathConfinement(t *testing.T) {
	t.Parallel()

	for _, test := range []struct{ name, input, output string }{
		{"source-output-overlap", "assets/**", "assets/generated"},
		{"source-cache-overlap", "generated/cache/**", "generated/assets"},
		{"cache-output-overlap", "assets/**", "generated/cache"},
		{"source-lock-overlap", "generated/cache/asset-build/**", "generated/assets"},
		{"lock-output-overlap", "assets/**", "generated/cache/asset-build"},
		{"escaping-output", "assets/**", "../escape"},
		{"escaping-input", "../assets/**", "generated/assets"},
		{"malformed-glob", "assets/[", "generated/assets"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			project := t.TempDir()

			config := Config{Input: []string{test.input}, Output: test.output}
			if _, err := Build(t.Context(), config, Options{Project: project, Identity: "paths"}); err == nil {
				t.Fatal("accepted invalid ownership")
			}
		})
	}

	project := t.TempDir()
	fixtureWrite(t, project, "generated/assets/keep.txt", []byte("keep"))

	var calls [3]int
	if _, err := Build(t.Context(), fixtureConfig(&calls), Options{Project: project, Identity: "unowned"}); err == nil {
		t.Fatal("accepted unowned nonempty output")
	}

	if calls != [3]int{} {
		t.Fatalf("processed before ownership validation: %v", calls)
	}

	data, err := fixtureRead(project, "generated/assets/keep.txt")
	if err != nil || string(data) != "keep" {
		t.Fatalf("unrelated output %q %v", data, err)
	}
}

func TestSymlinkSourceOutputAndCacheAreRejected(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"assets", "generated", "generated/cache", "generated/cache/assets/v1"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			project, target := t.TempDir(), t.TempDir()
			fixtureWrite(t, target, "source.txt", []byte("one"))

			full := filepath.Join(project, name)
			if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
				t.Fatal(err)
			}

			if err := os.Symlink(target, full); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}

			var calls [3]int
			if _, err := Build(t.Context(), fixtureConfig(&calls), Options{Project: project, Identity: "symlink"}); err == nil {
				t.Fatal("accepted symlink owner")
			}
		})
	}
}

func TestPublicationPrunesOwnedOutputsAndPreservesOthers(t *testing.T) {
	t.Parallel()
	project := t.TempDir()
	old := ownership{Version: cacheSchema, Outputs: []string{"stale.bin"}}

	marker, err := json.Marshal(old)
	if err != nil {
		t.Fatal(err)
	}

	fixtureWrite(t, project, "generated/assets/"+manifestName, marker)
	fixtureWrite(t, project, "generated/assets/stale.bin", []byte("stale"))
	fixtureWrite(t, project, "generated/assets/unrelated.txt", []byte("keep"))

	if _, err := Build(t.Context(), Config{Output: "generated/assets"}, Options{Project: project, Identity: "prune"}); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(project, "generated/assets/stale.bin")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale output survived: %v", err)
	}

	data, err := fixtureRead(project, "generated/assets/unrelated.txt")
	if err != nil || string(data) != "keep" {
		t.Fatalf("unrelated file changed %q %v", data, err)
	}
}

func TestInterruptedPublicationRecoversBeforeProcessing(t *testing.T) {
	t.Parallel()
	project := t.TempDir()
	journal := publication{
		Version: cacheSchema,
		Output:  "generated/assets",
		Old:     []string{"old.bin"},
		Files:   Bundle{"new.bin": []byte("complete")},
	}

	encoded, err := json.Marshal(journal)
	if err != nil {
		t.Fatal(err)
	}

	fixtureWrite(t, project, journalName, encoded)
	fixtureWrite(t, project, "generated/assets/old.bin", []byte("old"))
	fixtureWrite(t, project, "generated/assets/new.bin", []byte("partial"))

	wantErr := errStopAfterRecovery

	config := Config{Output: "generated/assets", Plugins: []Plugin{fixturePlugin(func(*Plan) error { return wantErr })}}
	if _, err := Build(t.Context(), config, Options{Project: project, Identity: "recover"}); !errors.Is(err, wantErr) {
		t.Fatalf("build: %v", err)
	}

	data, err := fixtureRead(project, "generated/assets/new.bin")
	if err != nil || string(data) != "complete" {
		t.Fatalf("recovered %q %v", data, err)
	}

	if _, err := os.Stat(filepath.Join(project, "generated/assets/old.bin")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale %v", err)
	}

	if _, err := os.Stat(filepath.Join(project, journalName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("journal %v", err)
	}
}

func TestProjectLockWaitIsCancelableAndReleased(t *testing.T) {
	t.Parallel()
	project := t.TempDir()

	unlock, err := acquireLock(t.Context(), project)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	if second, err := acquireLock(ctx, project); !errors.Is(err, context.DeadlineExceeded) {
		if second != nil {
			second()
		}

		t.Fatalf("lock wait %v", err)
	}

	unlock()

	release, err := acquireLock(t.Context(), project)
	if err != nil {
		t.Fatal(err)
	}

	release()
}

func TestPlanRejectsCyclesMissingSlotsAndUndeclaredReads(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name   string
		inputs []Ref
		read   Ref
	}{
		{"cycle", []Ref{{Action: "test", Slot: "files"}}, Ref{}},
		{"missing-producer", []Ref{{Action: "missing", Slot: "data"}}, Ref{}},
		{"missing-slot", []Ref{{Action: "source", Slot: "missing"}}, Ref{}},
		{"undeclared", nil, Ref{Action: "source", Slot: "data"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			project := t.TempDir()

			config := Config{Output: "generated/assets", Plugins: []Plugin{fixturePlugin(func(plan *Plan) error {
				_, err := plan.Add(
					Action{
						ID:        "source",
						Processor: "source",
						Revision:  "1",
						Outputs:   []Slot{{Name: "data", Schema: "text@1"}},
						Run:       func(*Context) (Result, error) { return Result{"data": []byte("source")}, nil },
					},
				)
				if err != nil {
					return err
				}

				refs, err := plan.Add(
					Action{
						ID:        "test",
						Processor: "test",
						Revision:  "1",
						Inputs:    test.inputs,
						Outputs:   []Slot{{Name: "files", Schema: "files@1"}},
						Run: func(ctx *Context) (Result, error) {
							if test.read.Action != "" {
								if _, err := ctx.Input(test.read); err != nil {
									return nil, err
								}
							}

							return Result{"files": []byte("{}")}, nil
						},
					},
				)
				if err == nil {
					plan.Publish(refs[0])
				}

				return err
			})}}
			if _, err := Build(t.Context(), config, Options{Project: project, Identity: "invalid-plan"}); err == nil {
				t.Fatal("accepted invalid plan")
			}
		})
	}
}

func TestConflictingGeneratedPathsFailBeforePublication(t *testing.T) {
	t.Parallel()
	project := t.TempDir()

	run, err := newRunner(Config{Output: "generated/assets"}, Options{Project: project, Identity: "conflict"})
	if err != nil {
		t.Fatal(err)
	}

	for _, files := range []Bundle{
		{"a": []byte("file"), "a-b": []byte("unrelated"), "a/b": []byte("child")},
		{manifestName + "/child": []byte("reserved parent")},
	} {
		if err := run.publish(files); err == nil {
			t.Fatal("accepted conflicting output paths")
		}

		if _, err := os.Stat(run.output); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("modified output before conflict rejection: %v", err)
		}

		if _, err := os.Stat(filepath.Join(project, journalName)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("wrote journal before conflict rejection: %v", err)
		}
	}
}

func TestTrackedQueriesRejectGeneratedAndCacheMembership(t *testing.T) {
	t.Parallel()
	project := t.TempDir()

	run, err := newRunner(Config{Output: "generated/assets"}, Options{Project: project, Identity: "queries"})
	if err != nil {
		t.Fatal(err)
	}

	plan := &Plan{Project: project, Output: run.output}

	ctx := &Context{Context: t.Context(), runner: run}
	for _, pattern := range []string{"**", "generated/**", "generated/cache/**", "generated/cache/asset-build/**"} {
		if _, err := ctx.Glob(pattern); err == nil {
			t.Errorf("action queried generated/cache state: %s", pattern)
		}

		if _, err := plan.Glob(pattern); err == nil {
			t.Errorf("plan queried generated/cache state: %s", pattern)
		}
	}

	for _, name := range []string{lockName, journalName, cacheDirectory + "/actions/entry"} {
		if _, err := ctx.ReadFile(name); err == nil {
			t.Errorf("action read generated state: %s", name)
		}

		if _, err := plan.ReadFile(name); err == nil {
			t.Errorf("plan read generated state: %s", name)
		}
	}
}
