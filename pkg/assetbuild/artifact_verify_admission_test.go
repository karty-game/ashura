//go:build !js

package assetbuild

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func artifactVerificationRunner(t *testing.T) *runner {
	t.Helper()

	return &runner{cache: t.TempDir(), noCache: true, artifacts: make(map[string]map[Artifact]bool)}
}

func writeVerificationArtifact(t *testing.T, run *runner, data []byte) Artifact {
	t.Helper()

	artifact := Artifact{SHA256: digest(data), Size: len(data)}
	fixtureWrite(t, run.cache, "artifacts/"+artifact.SHA256, data)

	return artifact
}

func TestArtifactStreamingFileParityAndDependencyClassification(t *testing.T) {
	t.Parallel()
	run := artifactVerificationRunner(t)
	data := []byte("immutable payload")

	artifact := writeVerificationArtifact(t, run, data)
	if matched, err := run.artifactMatches(artifact); err != nil || !matched {
		t.Fatal(matched, err)
	}

	if actual, err := run.readArtifact(artifact); err != nil || !slices.Equal(actual, data) {
		t.Fatal("owned artifact read changed", err)
	}

	dependency := dependency{Kind: artifactDependency, Name: artifact.SHA256, Value: "17"}
	if value, err := run.artifactDependencyValue(dependency); err != nil || value != dependency.Value {
		t.Fatal("valid dependency changed", value, err)
	}

	for _, changed := range [][]byte{data[:len(data)-1], append(append([]byte(nil), data...), 1), []byte("different payload")} {
		fixtureWrite(t, run.cache, "artifacts/"+artifact.SHA256, changed)

		if matched, err := run.artifactMatches(artifact); err != nil || matched {
			t.Fatal("changed file accepted", matched, err)
		}

		if value, err := run.artifactDependencyValue(dependency); err != nil || value != "corrupt" {
			t.Fatal("corrupt dependency semantics changed", value, err)
		}
	}

	if err := os.Remove(filepath.Join(run.cache, "artifacts", artifact.SHA256)); err != nil {
		t.Fatal(err)
	}

	if value, err := run.artifactDependencyValue(dependency); err != nil || value != missingDependency {
		t.Fatal("missing dependency semantics changed", value, err)
	}
}

func TestArtifactStreamingRejectsDirectoriesAndSymlinkComponents(t *testing.T) {
	t.Parallel()

	for _, kind := range []string{"directory", "file-symlink", "parent-symlink"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			run := artifactVerificationRunner(t)

			artifact := Artifact{SHA256: digest([]byte("payload")), Size: 7}

			switch kind {
			case "directory":
				if err := os.MkdirAll(filepath.Join(run.cache, "artifacts", artifact.SHA256), 0o700); err != nil {
					t.Fatal(err)
				}

			case "file-symlink", "parent-symlink":
				target := t.TempDir()
				fixtureWrite(t, target, artifact.SHA256, []byte("payload"))

				link := filepath.Join(run.cache, "artifacts")
				if kind == "file-symlink" {
					if err := os.MkdirAll(link, 0o700); err != nil {
						t.Fatal(err)
					}

					link = filepath.Join(link, artifact.SHA256)
					target = filepath.Join(target, artifact.SHA256)
				}

				if err := os.Symlink(target, link); err != nil {
					t.Skipf("symlink unavailable: %v", err)
				}
			}

			if matched, err := run.artifactMatches(artifact); matched || !errors.Is(err, errPath) {
				t.Fatal("unconfined/nonregular artifact accepted", matched, err)
			}
		})
	}
}

func TestArtifactStreamingRejectsOversizedFileBeforeReading(t *testing.T) {
	t.Parallel()

	run := artifactVerificationRunner(t)
	artifact := writeVerificationArtifact(t, run, nil)
	// Extend the logical length sparsely; no artifact-sized payload is created.
	if err := os.Truncate(filepath.Join(run.cache, "artifacts", artifact.SHA256), maxArtifactBytes+1); err != nil {
		t.Fatal(err)
	}

	if matched, err := run.artifactMatches(artifact); matched || !errors.Is(err, errPath) {
		t.Fatal("oversized cache file bypassed file admission", matched, err)
	}
}

func TestDeclaredArtifactStreamingAdmissionIsCanonicalCompleteAndCancelable(t *testing.T) {
	t.Parallel()
	run := artifactVerificationRunner(t)
	first := writeVerificationArtifact(t, run, []byte("first"))
	second := writeVerificationArtifact(t, run, []byte("second"))

	var plan Plan

	called := false

	_, err := plan.Add(
		Action{
			ID:        "verify",
			Processor: "verify",
			Revision:  "1",
			Artifacts: []Artifact{second, first, second},
			Outputs:   []Slot{{Name: "data", Schema: "fixture@1"}},
			Run: func(ctx *Context) (Result, error) {
				called = true

				for _, artifact := range []Artifact{first, second} {
					if !ctx.artifacts[artifact] {
						t.Fatal("Run started before complete dependency admission")
					}
				}

				if len(ctx.deps) != 2 {
					t.Fatal("canonical duplicates changed tracked dependencies", ctx.deps)
				}

				return Result{"data": []byte("ok")}, nil
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	action := plan.actions[0]
	if _, err = run.runAction(t.Context(), action, "unused"); err != nil || !called {
		t.Fatal("valid declared roster rejected", called, err)
	}

	called = false

	fixtureWrite(t, run.cache, "artifacts/"+first.SHA256, []byte("wrong"))

	if _, err = run.runAction(t.Context(), action, "unused"); !errors.Is(err, errPlan) || called {
		t.Fatal("corrupt declared artifact reached Run", called, err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err = run.runAction(ctx, action, "unused"); !errors.Is(err, context.Canceled) || called {
		t.Fatal("canceled admission reached Run", called, err)
	}

	if err = run.admitDeclaredArtifacts(ctx, &Context{Context: ctx, runner: run}); !errors.Is(err, context.Canceled) {
		t.Fatal("empty admission ignored cancellation", err)
	}
}

func TestDeclaredArtifactStreamingReturnsCanonicalFirstFailure(t *testing.T) {
	t.Parallel()
	run := artifactVerificationRunner(t)
	first, last := strings.Repeat("0", 64), strings.Repeat("f", 64)

	var plan Plan

	_, err := plan.Add(
		Action{
			ID:        "bad",
			Processor: "bad",
			Revision:  "1",
			Artifacts: []Artifact{{SHA256: last, Size: 1}, {SHA256: first, Size: 1}},
			Outputs:   []Slot{{Name: "data", Schema: "fixture@1"}},
			Run: func(*Context) (Result, error) {
				t.Fatal("missing declared artifact reached Run")

				return nil, errFixtureFailure
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	for range 8 {
		_, err = run.runAction(t.Context(), plan.actions[0], "unused")
		if !errors.Is(err, os.ErrNotExist) || !strings.Contains(err.Error(), first) || strings.Contains(err.Error(), last) {
			t.Fatal("parallel admission changed canonical error precedence", err)
		}
	}
}
