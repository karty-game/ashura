//go:build !js

package assetbuild

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
)

func TestConcurrentExecutionStableRuntimeFailurePrecedence(t *testing.T) {
	t.Parallel()
	project := t.TempDir()
	concurrentExecutionPublishOld(t, project)

	early, late := errConcurrentEarly, errConcurrentLate
	laterStarted := make(chan struct{})
	trace := func(event ActionEvent) {
		if event.Action == "later" && event.Phase == ActionFinished {
			close(laterStarted)
		}
	}

	var active atomic.Int32

	config := Config{Output: "generated/assets", Plugins: []Plugin{fixturePlugin(func(plan *Plan) error {
		plan.Cleanup(func() error {
			if active.Load() != 0 {
				return errConcurrentCleanup
			}

			return nil
		})

		for index := range 2 {
			_, err := plan.Add(
				Action{
					ID:        []string{"earlier", "later"}[index],
					Processor: "failure",
					Revision:  "1",
					Parallel:  true,
					Outputs:   []Slot{{Name: "data", Schema: "data@1"}},
					Run: func(ctx *Context) (Result, error) {
						active.Add(1)

						defer active.Add(-1)

						if index == 0 {
							select {
							case <-laterStarted:
								return nil, early
							case <-ctx.Done():
								return nil, ctx.Err()
							}
						}

						return nil, late
					},
				},
			)
			if err != nil {
				return err
			}
		}

		return nil
	})}}

	_, err := Build(t.Context(), config, Options{Project: project, Identity: "failures", Jobs: 2, Trace: trace})
	if !errors.Is(err, early) || errors.Is(err, late) {
		t.Fatal("runtime failure order changed", err)
	}

	assertConcurrentExecutionOld(t, project)
}

func TestConcurrentExecutionCancellationJoinsBeforeCleanup(t *testing.T) {
	t.Parallel()
	project := t.TempDir()
	concurrentExecutionPublishOld(t, project)

	ctx, cancel := context.WithCancel(t.Context())

	defer cancel()

	entered := make(chan struct{}, 2)

	var active atomic.Int32

	config := Config{Output: "generated/assets", Plugins: []Plugin{fixturePlugin(func(plan *Plan) error {
		plan.Cleanup(func() error {
			if active.Load() != 0 {
				return errConcurrentCleanup
			}

			return nil
		})

		for _, name := range []string{"cpu", "gpu"} {
			_, err := plan.Add(
				Action{
					ID:        name,
					Processor: "wait",
					Revision:  "1",
					Parallel:  true,
					Outputs:   []Slot{{Name: "data", Schema: "data@1"}},
					Run: func(ctx *Context) (Result, error) {
						active.Add(1)

						defer active.Add(-1)

						entered <- struct{}{}

						<-ctx.Done()

						return nil, ctx.Err()
					},
				},
			)
			if err != nil {
				return err
			}
		}

		return nil
	})}}

	go func() { <-entered; <-entered; cancel() }()

	_, err := Build(ctx, config, Options{Project: project, Identity: "cancel", Jobs: 2})
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}

	assertConcurrentExecutionOld(t, project)
}

func invalidConcurrentAction(kind string, index int, identity string, ran *atomic.Int32) Action {
	action := Action{
		ID:        identity,
		Processor: "invalid",
		Revision:  "1",
		Parallel:  true,
		Outputs:   []Slot{{Name: "data", Schema: "data@1"}},
		Run: func(*Context) (Result, error) {
			ran.Add(1)

			return Result{"data": []byte("valid")}, nil
		},
	}

	switch kind {
	case "missing-slot":
		if index == 1 {
			action.Inputs = []Ref{{Action: "first", Slot: "missing"}}
		}

	case "cycle":
		action.Inputs = []Ref{{Action: []string{"second", "first"}[index], Slot: "data"}}
	case "artifact":
		action.Artifacts = []Artifact{{SHA256: strings.Repeat("0", 64), Size: 0}}
	}

	return action
}

func TestAllRecipeFormsAdmitCompleteGraphAndArtifacts(t *testing.T) {
	t.Parallel()

	for _, kind := range []string{"missing-slot", "cycle", "artifact"} {
		for _, jobs := range []int{0, 1, 2} {
			for _, named := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/jobs=%d/named=%t", kind, jobs, named), func(t *testing.T) {
					t.Parallel()
					project := t.TempDir()
					concurrentExecutionPublishOld(t, project)

					var ran atomic.Int32

					config := invalidGraphFixtureConfig(kind, named, &ran)
					if _, err := Build(t.Context(), config, Options{Project: project, Identity: "invalid", Jobs: jobs}); err == nil {
						t.Fatal("invalid graph/artifact accepted")
					}

					if ran.Load() != 0 {
						t.Fatal("partial graph executed before complete admission")
					}

					assertConcurrentExecutionOld(t, project)
				})
			}
		}
	}
}

func invalidGraphFixtureConfig(kind string, named bool, ran *atomic.Int32) Config {
	plugins := []Plugin{fixturePlugin(func(plan *Plan) error {
		for index, identity := range []string{"first", "second"} {
			if _, err := plan.Add(invalidConcurrentAction(kind, index, identity, ran)); err != nil {
				return err
			}
		}

		return nil
	})}

	return admissionFixtureConfig(plugins, named)
}

func admissionFixtureConfig(plugins []Plugin, named bool) Config {
	if named {
		return Config{Output: "generated/assets", Pipelines: []Pipeline{{Name: "invalid", Import: plugins}}}
	}

	return Config{Output: "generated/assets", Plugins: plugins}
}

func TestConcurrentExecutionDefaultAndSerialActionsStaySerial(t *testing.T) {
	t.Parallel()

	for _, jobs := range []int{0, 1, 2} {
		var active, maximum atomic.Int32

		config := Config{Output: "generated/assets", Plugins: []Plugin{fixturePlugin(func(plan *Plan) error {
			for _, name := range []string{"one", "two"} {
				_, err := plan.Add(
					Action{
						ID:        name,
						Processor: "serial",
						Revision:  "1",
						Parallel:  jobs != 2,
						Outputs:   []Slot{{Name: "data", Schema: "data@1"}},
						Run: func(*Context) (Result, error) {
							count := active.Add(1)
							maximum.Store(max(maximum.Load(), count))
							active.Add(-1)

							return Result{"data": nil}, nil
						},
					},
				)
				if err != nil {
					return err
				}
			}

			return nil
		})}}
		if _, err := Build(
			t.Context(),
			config,
			Options{Project: t.TempDir(), Identity: "serial", Jobs: jobs},
		); err != nil ||
			maximum.Load() != 1 {
			t.Fatal("default serial contract changed", jobs, maximum.Load(), err)
		}
	}
}

func TestConcurrentExecutionRejectsUnrelatedArtifactProvenance(t *testing.T) {
	t.Parallel()
	project := t.TempDir()
	concurrentExecutionPublishOld(t, project)

	secret := []byte("secret")

	config := Config{Output: "generated/assets", Plugins: []Plugin{fixturePlugin(func(plan *Plan) error {
		_, err := plan.Add(
			Action{
				ID:        "unrelated-source",
				Processor: "secret",
				Revision:  "1",
				Outputs:   []Slot{{Name: "data", Schema: "secret@1"}},
				Run: func(ctx *Context) (Result, error) {
					artifact, err := ctx.StoreArtifact(secret)
					if err != nil {
						return nil, err
					}

					encoded, err := json.Marshal(artifact)

					return Result{"data": encoded}, err
				},
			},
		)
		if err != nil {
			return err
		}

		_, err = plan.Add(
			Action{
				ID:        "reader",
				Processor: "reader",
				Revision:  "1",
				Parallel:  true,
				Outputs:   []Slot{{Name: "data", Schema: "data@1"}},
				Run: func(ctx *Context) (Result, error) {
					_, err := ctx.ReadArtifact(Artifact{SHA256: digest(secret), Size: len(secret)})

					return nil, err
				},
			},
		)

		return err
	})}}
	if _, err := Build(
		t.Context(),
		config,
		Options{Project: project, Identity: "provenance", Jobs: 2},
	); err == nil ||
		!strings.Contains(err.Error(), "not declared upstream") {
		t.Fatal("parallel view authorized unrelated artifact", err)
	}

	assertConcurrentExecutionOld(t, project)
}

func TestRecipeAdmissionIncludesExpansionAndPublicationReferences(t *testing.T) {
	t.Parallel()

	for _, kind := range []string{"expansion", "publication"} {
		for _, named := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/named=%t", kind, named), func(t *testing.T) {
				t.Parallel()

				var ran atomic.Int32

				plugins := []Plugin{fixturePlugin(func(plan *Plan) error {
					if _, err := plan.Add(invalidConcurrentAction("", 0, "first", &ran)); err != nil {
						return err
					}

					missing := Ref{Action: "first", Slot: "missing"}
					if kind == "publication" {
						plan.Publish(missing)

						return nil
					}

					return plan.Expand(Expansion{ID: "invalid", Inputs: []Ref{missing},
						Run: func(*Context, *Plan) error {
							ran.Add(1)

							return nil
						}})
				})}

				config := admissionFixtureConfig(plugins, named)

				if _, err := Build(t.Context(), config, Options{Project: t.TempDir(), Identity: "invalid"}); !errors.Is(err, errPlan) {
					t.Fatalf("invalid %s reference admitted: %v", kind, err)
				}

				if ran.Load() != 0 {
					t.Fatal("processor or expansion ran before complete graph admission")
				}
			})
		}
	}
}
