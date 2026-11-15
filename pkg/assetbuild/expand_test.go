//go:build !js

package assetbuild

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

type fanoutItem struct {
	Name string   `json:"name"`
	Data Artifact `json:"data"`
}

func fanoutConfig(calls map[string]int, expansion func(*Context, *Plan) error) Config {
	return Config{Input: []string{"assets/**"}, Output: "generated/assets", Plugins: []Plugin{fixturePlugin(func(plan *Plan) error {
		refs, err := plan.Add(Action{ID: "discovery", Processor: "discover", Revision: "1",
			Outputs: []Slot{{Name: "manifest", Schema: "manifest@1"}}, Run: fanoutManifest(calls)})
		if err != nil {
			return err
		}

		manifest := refs[0]

		return plan.Expand(Expansion{ID: "discover-chunks", Inputs: []Ref{manifest}, Run: func(ctx *Context, child *Plan) error {
			calls["expansion"]++

			data, err := ctx.Input(manifest)
			if err != nil {
				return err
			}

			var items []fanoutItem
			if err := json.Unmarshal(data, &items); err != nil {
				return err
			}

			for _, item := range items {
				refs, err := child.Add(Action{ID: "chunk/" + item.Name, Processor: "chunk", Revision: "1",
					Artifacts: []Artifact{item.Data}, Outputs: []Slot{{Name: "files", Schema: "files@1"}},
					Run: func(ctx *Context) (Result, error) {
						calls[item.Name]++

						data, err := ctx.ReadArtifact(item.Data)
						if err != nil {
							return nil, err
						}

						bundle, err := EncodeBundle(Bundle{item.Name + ".bin": data})

						return Result{"files": bundle}, err
					}})
				if err != nil {
					return err
				}

				child.Publish(refs[0])
				child.Resources = append(child.Resources, Resource{ID: item.Name, Ref: refs[0]})
			}

			if expansion != nil {
				return expansion(ctx, child)
			}

			return nil
		}})
	})}}
}

func fanoutManifest(calls map[string]int) func(*Context) (Result, error) {
	return func(ctx *Context) (Result, error) {
		calls["discovery"]++

		data, err := ctx.ReadFile("assets/roster.txt")
		if err != nil {
			return nil, err
		}

		items := make([]fanoutItem, 0)

		for name := range strings.FieldsSeq(string(data)) {
			artifact, err := ctx.StoreArtifact([]byte("payload:" + name))
			if err != nil {
				return nil, err
			}

			items = append(items, fanoutItem{Name: name, Data: artifact})
		}

		encoded, err := json.Marshal(items)

		return Result{"manifest": encoded}, err
	}
}

func TestExpansionCachesPerChunkAndDiscoversMembership(t *testing.T) {
	t.Parallel()
	project := t.TempDir()
	fixtureWrite(t, project, "assets/roster.txt", []byte("a b"))

	calls := make(map[string]int)
	config := fanoutConfig(calls, func(_ *Context, child *Plan) error {
		return child.Expand(Expansion{ID: "verify-resources", Run: func(_ *Context, nested *Plan) error {
			if len(nested.Resources) == 0 {
				return errFixtureFailure
			}

			for _, resource := range nested.Resources {
				if _, err := nested.SlotSchema(resource.Ref); err != nil {
					return err
				}
			}

			return nil
		}})
	})

	options := Options{Project: project, Identity: "fanout"}
	for iteration := range 2 {
		report, err := Build(t.Context(), config, options)
		if err != nil {
			t.Fatal(err)
		}

		if iteration == 0 && report.Executed != 3 || iteration == 1 && report.Reused != 3 {
			t.Fatalf("iteration %d: %+v", iteration, report)
		}
	}

	fixtureWrite(t, project, "assets/roster.txt", []byte("a c"))

	report, err := Build(t.Context(), config, options)
	if err != nil {
		t.Fatal(err)
	}

	if report.Executed != 2 || report.Reused != 1 || calls["a"] != 1 || calls["b"] != 1 || calls["c"] != 1 || calls["expansion"] != 3 {
		t.Fatalf("membership did not preserve unchanged chunk: %+v %v", report, calls)
	}

	if _, err := os.Stat(filepath.Join(project, "generated/assets/b.bin")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("removed chunk survived: %v", err)
	}

	for _, name := range []string{"a", "c"} {
		data, err := fixtureRead(project, "generated/assets/"+name+".bin")
		if err != nil || string(data) != "payload:"+name {
			t.Fatalf("chunk %s: %q %v", name, data, err)
		}
	}
}

func TestArtifactsMissingOrCorruptRebuildProducer(t *testing.T) {
	t.Parallel()

	for _, corrupt := range []bool{false, true} {
		t.Run(strconv.FormatBool(corrupt), func(t *testing.T) {
			t.Parallel()
			project := t.TempDir()
			fixtureWrite(t, project, "assets/roster.txt", []byte("a"))

			calls := make(map[string]int)
			config := fanoutConfig(calls, nil)

			options := Options{Project: project, Identity: "artifact-recovery"}
			if _, err := Build(t.Context(), config, options); err != nil {
				t.Fatal(err)
			}

			file := filepath.Join(project, cacheDirectory, "artifacts", digest([]byte("payload:a")))
			if corrupt {
				if err := os.WriteFile(file, []byte("tampered"), 0o600); err != nil {
					t.Fatal(err)
				}
			} else if err := os.Remove(file); err != nil {
				t.Fatal(err)
			}

			report, err := Build(t.Context(), config, options)
			if err != nil {
				t.Fatal(err)
			}

			if report.Executed != 1 || calls["discovery"] != 2 || calls["a"] != 1 {
				t.Fatalf("artifact recovery %+v %v", report, calls)
			}

			data, err := fixtureRead(project, cacheDirectory+"/artifacts/"+digest([]byte("payload:a")))
			if err != nil || string(data) != "payload:a" {
				t.Fatalf("bad repaired artifact %q %v", data, err)
			}
		})
	}
}

func TestExpansionFailuresPreservePublication(t *testing.T) {
	t.Parallel()

	for _, mode := range []string{"callback", "duplicate", "unknown-ref", "source-read", "environment-read", "artifact-store", "cycle", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			project := t.TempDir()
			fixtureWrite(t, project, "assets/roster.txt", []byte("a"))
			options := Options{Project: project, Identity: mode}

			calls := make(map[string]int)
			if _, err := Build(t.Context(), fanoutConfig(calls, nil), options); err != nil {
				t.Fatal(err)
			}

			fixtureWrite(t, project, "assets/roster.txt", []byte("b"))

			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()

			config := fanoutConfig(calls, func(context *Context, child *Plan) error {
				return failFanout(mode, cancel, context, child)
			})
			if _, err := Build(ctx, config, options); err == nil {
				t.Fatal("accepted failed discovery")
			}

			if mode == "cycle" && calls["b"] != 0 {
				t.Fatal("executed child action before admitting the complete discovered graph")
			}

			old, err := fixtureRead(project, "generated/assets/a.bin")
			if err != nil || string(old) != "payload:a" {
				t.Fatalf("old generation changed %q %v", old, err)
			}

			if _, err := os.Stat(filepath.Join(project, "generated/assets/b.bin")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("new output leaked: %v", err)
			}
		})
	}
}

func TestBuildCleanupLIFOAndFailures(t *testing.T) {
	t.Parallel()

	for _, phase := range []string{"success", "plan", "expansion", "action"} {
		t.Run(phase, func(t *testing.T) {
			t.Parallel()

			var order []int

			config := Config{Output: "generated/assets", Plugins: []Plugin{fixturePlugin(func(plan *Plan) error {
				plan.Cleanup(func() error {
					order = append(order, 1)

					return errFixtureFailure
				})

				if phase == "plan" {
					return errFixtureFailure
				}

				return plan.Expand(Expansion{ID: "cleanup", Run: func(_ *Context, child *Plan) error {
					child.Cleanup(func() error {
						order = append(order, 2)

						return nil
					})

					if phase == "expansion" {
						return errFixtureFailure
					}

					_, err := child.Add(
						Action{
							ID:        "work",
							Processor: "work",
							Revision:  "1",
							Outputs:   []Slot{{Name: "data", Schema: "text@1"}},
							Run: func(*Context) (Result, error) {
								if phase == "action" {
									return nil, errStopAfterRecovery
								}

								return Result{"data": nil}, nil
							},
						},
					)

					return err
				}})
			})}}

			_, err := Build(t.Context(), config, Options{Project: t.TempDir(), Identity: phase})
			if !errors.Is(err, errFixtureFailure) {
				t.Fatalf("cleanup error lost: %v", err)
			}

			if phase == "action" && !errors.Is(err, errStopAfterRecovery) {
				t.Fatalf("processing error lost: %v", err)
			}

			want := []int{2, 1}
			if phase == "plan" {
				want = []int{1}
			}

			if !slices.Equal(order, want) {
				t.Fatalf("cleanup order %v want%v", order, want)
			}
		})
	}
}

func failFanout(mode string, cancel context.CancelFunc, context *Context, child *Plan) error {
	switch mode {
	case "callback":
		return errFixtureFailure
	case "duplicate":
		return child.Expand(Expansion{ID: "discovery", Run: func(*Context, *Plan) error { return nil }})
	case "unknown-ref":
		child.Publish(Ref{Action: "absent", Slot: "files"})
	case "source-read":
		_, _ = context.ReadFile("assets/roster.txt")
	case "environment-read":
		_ = context.Getenv("PATH")
	case "artifact-store":
		_, _ = context.StoreArtifact([]byte("forbidden"))
	case "cancel":
		cancel()
	case "cycle":
		for _, pair := range [][2]string{{"cycle-a", "cycle-b"}, {"cycle-b", "cycle-a"}} {
			_, err := child.Add(
				Action{
					ID:        pair[0],
					Processor: "cycle",
					Revision:  "1",
					Inputs:    []Ref{{Action: pair[1], Slot: "data"}},
					Outputs: []Slot{
						{Name: "data", Schema: "text@1"},
					},
					Run: func(*Context) (Result, error) { return Result{"data": nil}, nil },
				},
			)
			if err != nil {
				return err
			}
		}
	}

	return nil
}
