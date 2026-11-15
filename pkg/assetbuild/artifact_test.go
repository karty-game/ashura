//go:build !js

package assetbuild

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestArtifactAdmissionAndProvenance(t *testing.T) {
	t.Parallel()

	for _, artifact := range []Artifact{
		{SHA256: "../escape", Size: 0},
		{SHA256: digest(nil), Size: -1},
		{SHA256: digest(nil), Size: maxArtifactBytes + 1},
	} {
		plan := &Plan{}
		if _, err := plan.Add(Action{
			ID:        "invalid",
			Processor: "invalid",
			Revision:  "1",
			Artifacts: []Artifact{artifact},
			Outputs: []Slot{
				{Name: "data", Schema: "test@1"},
			},
			Run: func(*Context) (Result, error) { return Result{"data": nil}, nil },
		}); err == nil {
			t.Fatalf("admitted invalid artifact %+v", artifact)
		}
	}

	project := t.TempDir()
	fixtureWrite(t, project, "assets/roster.txt", []byte("a"))

	config := fanoutConfig(make(map[string]int), func(ctx *Context, child *Plan) error {
		manifest, err := ctx.Input(Ref{Action: "discovery", Slot: "manifest"})
		if err != nil {
			return err
		}

		var items []fanoutItem
		if err := json.Unmarshal(manifest, &items); err != nil {
			return err
		}

		if _, err := ctx.ReadArtifact(items[0].Data); err != nil {
			return err
		}

		_, err = child.Add(
			Action{ID: "undeclared", Processor: "undeclared", Revision: "1", Outputs: []Slot{{Name: "data", Schema: "test@1"}},
				Run: func(ctx *Context) (Result, error) {
					_, err := ctx.ReadArtifact(items[0].Data)

					return Result{"data": nil}, err
				}},
		)

		return err
	})
	if _, err := Build(t.Context(), config, Options{Project: project, Identity: "artifact-provenance"}); err == nil {
		t.Fatal("undeclared artifact read succeeded")
	}
}

func TestArtifactSnapshotsAndNoCache(t *testing.T) {
	t.Parallel()
	project := t.TempDir()

	var artifact Artifact

	config := Config{Output: "generated/assets", Plugins: []Plugin{fixturePlugin(func(plan *Plan) error {
		refs, err := plan.Add(
			Action{
				ID:        "source",
				Processor: "source",
				Revision:  "1",
				Outputs:   []Slot{{Name: "manifest", Schema: "test@1"}},
				Run: func(ctx *Context) (Result, error) {
					data := []byte("original")

					var err error

					artifact, err = ctx.StoreArtifact(data)
					if err != nil {
						return nil, err
					}

					data[0] = 'X'
					encoded, err := json.Marshal(artifact)

					return Result{"manifest": encoded}, err
				},
			},
		)
		if err != nil {
			return err
		}

		_, err = plan.Add(
			Action{
				ID:        "consumer",
				Processor: "consumer",
				Revision:  "1",
				Inputs:    refs,
				Outputs:   []Slot{{Name: "data", Schema: "test@1"}},
				Run: func(ctx *Context) (Result, error) {
					data, err := ctx.ReadArtifact(artifact)
					if err != nil {
						return nil, err
					}

					data[0] = 'X'

					again, err := ctx.ReadArtifact(artifact)
					if err != nil {
						return nil, err
					}

					if string(again) != "original" {
						return nil, errFixtureFailure
					}

					return Result{"data": again}, nil
				},
			},
		)

		return err
	})}}
	for range 2 {
		if _, err := Build(t.Context(), config, Options{Project: project, Identity: "artifact-snapshot", NoCache: true}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestArtifactSymlinkCacheIsRejected(t *testing.T) {
	t.Parallel()
	project := t.TempDir()
	fixtureWrite(t, project, "assets/roster.txt", []byte("a"))

	config := fanoutConfig(make(map[string]int), nil)

	options := Options{Project: project, Identity: "artifact-symlink"}
	if _, err := Build(t.Context(), config, options); err != nil {
		t.Fatal(err)
	}

	artifacts := filepath.Join(project, cacheDirectory, "artifacts")
	if err := os.RemoveAll(artifacts); err != nil {
		t.Fatal(err)
	}

	target := t.TempDir()
	fixtureWrite(t, target, digest([]byte("payload:a")), []byte("payload:a"))

	if err := os.Symlink(target, artifacts); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	if _, err := Build(t.Context(), config, options); !errors.Is(err, errPath) {
		t.Fatalf("symlink not rejected: %v", err)
	}

	data, err := fixtureRead(project, "generated/assets/a.bin")
	if err != nil || string(data) != "payload:a" {
		t.Fatalf("publication changed %q %v", data, err)
	}
}

func TestDeclaredArtifactOrderDoesNotInvalidateCache(t *testing.T) {
	t.Parallel()
	project := t.TempDir()

	var (
		reverse bool
		calls   int
	)

	config := Config{Output: "generated/assets", Plugins: []Plugin{fixturePlugin(func(plan *Plan) error {
		refs, err := plan.Add(Action{ID: "pair", Processor: "pair", Revision: "1", Outputs: []Slot{{Name: "manifest", Schema: "pair@1"}},
			Run: func(ctx *Context) (Result, error) {
				first, err := ctx.StoreArtifact([]byte("first"))
				if err != nil {
					return nil, err
				}

				second, err := ctx.StoreArtifact([]byte("second"))
				if err != nil {
					return nil, err
				}

				encoded, err := json.Marshal([]Artifact{first, second})

				return Result{"manifest": encoded}, err
			}})
		if err != nil {
			return err
		}

		return plan.Expand(Expansion{ID: "pair-expand", Inputs: refs, Run: func(ctx *Context, child *Plan) error {
			data, err := ctx.Input(refs[0])
			if err != nil {
				return err
			}

			var artifacts []Artifact
			if err := json.Unmarshal(data, &artifacts); err != nil {
				return err
			}

			if reverse {
				slices.Reverse(artifacts)
				artifacts = append(artifacts, artifacts[0])
			}

			_, err = child.Add(Action{ID: "pair-consumer", Processor: "pair-consumer", Revision: "1", Artifacts: artifacts,
				Outputs: []Slot{{Name: "done", Schema: "test@1"}}, Run: func(*Context) (Result, error) {
					calls++

					return Result{"done": nil}, nil
				}})

			return err
		}})
	})}}

	options := Options{Project: project, Identity: "artifact-set"}
	if _, err := Build(t.Context(), config, options); err != nil {
		t.Fatal(err)
	}

	reverse = true

	report, err := Build(t.Context(), config, options)
	if err != nil {
		t.Fatal(err)
	}

	if calls != 1 || report.Reused != 2 {
		t.Fatalf("artifact order changed identity: %+v calls%d", report, calls)
	}
}
