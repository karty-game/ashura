//go:build !js

package assetbuild

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

const pipelineTestDeadline = 5 * time.Second

func pipelineActionBranch(name string, action Action) Pipeline {
	return Pipeline{Name: name, Process: []Plugin{fixturePlugin(func(plan *Plan) error {
		_, err := plan.Add(action)

		return err
	})}}
}

func pipelineWait(ctx context.Context, signal <-chan struct{}) error {
	select {
	case <-signal:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestPipelineUIExpansionCompletesWhileWorldActionBlocks(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(t.Context(), pipelineTestDeadline)
	defer cancel()

	worldStarted, uiFinished := make(chan struct{}), make(chan struct{})
	world := pipelineFixtureAction("work")
	world.Parallel = true
	world.Run = func(ctx *Context) (Result, error) {
		close(worldStarted)

		return Result{"data": []byte("world")}, pipelineWait(ctx, uiFinished)
	}
	uiBranch := Pipeline{Name: "ui", Import: []Plugin{fixturePlugin(func(plan *Plan) error {
		manifest := pipelineFixtureAction("manifest")
		manifest.Parallel = true

		manifest.Run = func(ctx *Context) (Result, error) {
			return Result{"data": []byte("ui")}, pipelineWait(ctx, worldStarted)
		}
		if _, err := plan.Add(manifest); err != nil {
			return err
		}

		local := Ref{Action: "manifest", Slot: "data"}

		return plan.Expand(Expansion{ID: "discover", Inputs: []Ref{local}, Run: func(ctx *Context, child *Plan) error {
			data, err := ctx.Input(local)
			if err != nil || string(data) != "ui" {
				return errors.Join(errFixtureFailure, err)
			}

			export := pipelineFixtureAction("export")
			export.Parallel, export.Inputs = true, []Ref{local}

			export.Run = func(ctx *Context) (Result, error) {
				data, err := ctx.Input(local)
				if err != nil {
					return nil, err
				}

				bundle, err := EncodeBundle(Bundle{"ui.txt": data})

				return Result{"data": bundle}, err
			}
			if _, err := child.Add(export); err != nil {
				return err
			}

			child.Publish(Ref{Action: "export", Slot: "data"})

			return nil
		}})
	})}}
	project := t.TempDir()
	trace := func(event ActionEvent) {
		if event.Action == "pipeline/ui/export" && event.Phase == ActionFinished {
			close(uiFinished)
		}
	}

	config := Config{Output: "generated/assets", Pipelines: []Pipeline{pipelineActionBranch("world", world), uiBranch}}
	if _, err := Build(ctx, config, Options{Project: project, Identity: "pipeline-progress", Jobs: 2, Trace: trace}); err != nil {
		t.Fatal(err)
	}

	if data, err := fixtureRead(project, "generated/assets/ui.txt"); err != nil || string(data) != "ui" {
		t.Fatalf("UI publication: %q %v", data, err)
	}
}

func TestPipelineDefaultJobsPreserveBranchOrder(t *testing.T) {
	t.Parallel()

	for _, jobs := range []int{0, 1} {
		t.Run(strconv.Itoa(jobs), func(t *testing.T) {
			t.Parallel()

			var order []string

			config := Config{Output: "generated/assets"}

			for _, name := range []string{"first", "second"} {
				action := pipelineFixtureAction("work")
				action.Parallel = true
				action.Run = func(*Context) (Result, error) {
					order = append(order, name)

					return Result{"data": nil}, nil
				}
				config.Pipelines = append(config.Pipelines, pipelineActionBranch(name, action))
			}

			if _, err := Build(t.Context(), config, Options{Project: t.TempDir(), Identity: "pipeline-serial", Jobs: jobs}); err != nil {
				t.Fatal(err)
			}

			if !slices.Equal(order, []string{"first", "second"}) {
				t.Fatalf("serial branch order: %v", order)
			}
		})
	}
}

func TestPipelineGlobalJobsTokensAndUnmarkedExclusion(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(t.Context(), pipelineTestDeadline)
	defer cancel()

	const jobs = 2

	entered, release := make(chan struct{}, jobs), make(chan struct{})
	shared := NewExecutionResource()
	definitions := []struct {
		name     string
		parallel bool
		token    *ExecutionResource
	}{{"tokenA", true, shared}, {"tokenB", true, shared}, {"free", true, nil}, {"serial", false, nil}}
	config := Config{Output: "generated/assets"}

	for _, definition := range definitions {
		action := pipelineFixtureAction("work")
		action.Parallel, action.Exclusive = definition.parallel, definition.token
		action.Run = func(ctx *Context) (Result, error) {
			select {
			case entered <- struct{}{}:
			default:
			}

			return Result{"data": nil}, pipelineWait(ctx, release)
		}
		config.Pipelines = append(config.Pipelines, pipelineActionBranch(definition.name, action))
	}

	active := make(map[string]bool)
	maximum := 0

	var violations []string

	trace := func(event ActionEvent) {
		if event.Phase == ActionFinished {
			delete(active, event.Action)

			return
		}

		serial := "pipeline/serial/work"
		if active[serial] || event.Action == serial && len(active) != 0 {
			violations = append(violations, "unmarked action overlapped")
		}

		if event.Action == "pipeline/tokenA/work" && active["pipeline/tokenB/work"] ||
			event.Action == "pipeline/tokenB/work" && active["pipeline/tokenA/work"] {
			violations = append(violations, "shared token overlapped")
		}

		active[event.Action] = true
		maximum = max(maximum, len(active))
	}

	controllerDone := make(chan struct{})
	go func() {
		defer close(controllerDone)

		for range jobs {
			if pipelineWait(ctx, entered) != nil {
				return
			}
		}

		close(release)
	}()

	_, err := Build(ctx, config, Options{Project: t.TempDir(), Identity: "pipeline-resources", Jobs: jobs, Trace: trace})

	cancel()
	<-controllerDone

	if err != nil || maximum != jobs || len(violations) != 0 || len(active) != 0 {
		t.Fatalf("global admission: max=%d active=%v violations=%v error=%v", maximum, active, violations, err)
	}
}

func pipelineCleanupBranch(name string, action Action, active, cleanups *atomic.Int32) Pipeline {
	branch := pipelineActionBranch(name, action)
	branch.Import = []Plugin{fixturePlugin(func(plan *Plan) error {
		plan.Cleanup(func() error {
			cleanups.Add(1)

			if active.Load() != 0 {
				return errConcurrentCleanup
			}

			return nil
		})

		return nil
	})}

	return branch
}

func TestPipelineFailurePrecedenceAndJoinedCleanup(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(t.Context(), pipelineTestDeadline)
	defer cancel()

	project := t.TempDir()
	concurrentExecutionPublishOld(t, project)

	laterFinished := make(chan struct{})

	var active, cleanups atomic.Int32

	config := Config{Output: "generated/assets"}

	for index, name := range []string{"earlier", "later"} {
		action := pipelineFixtureAction("work")
		action.Parallel = true
		action.Run = func(ctx *Context) (Result, error) {
			active.Add(1)
			defer active.Add(-1)

			if index == 0 {
				if err := pipelineWait(ctx, laterFinished); err != nil {
					return nil, err
				}

				return nil, errConcurrentEarly
			}

			return nil, errConcurrentLate
		}
		config.Pipelines = append(config.Pipelines, pipelineCleanupBranch(name, action, &active, &cleanups))
	}

	trace := func(event ActionEvent) {
		if event.Action == "pipeline/later/work" && event.Phase == ActionFinished {
			close(laterFinished)
		}
	}

	_, err := Build(ctx, config, Options{Project: project, Identity: "pipeline-failures", Jobs: 2, Trace: trace})
	if !errors.Is(err, errConcurrentEarly) || errors.Is(err, errConcurrentLate) || errors.Is(err, errConcurrentCleanup) ||
		cleanups.Load() != 2 {
		t.Fatalf("failure precedence/cleanup: %v; cleanups=%d", err, cleanups.Load())
	}

	assertConcurrentExecutionOld(t, project)
}

func TestPipelineCancellationJoinsAllBranchesBeforeCleanup(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(t.Context(), pipelineTestDeadline)
	defer cancel()

	const jobs = 2

	entered := make(chan struct{}, jobs)

	var active, cleanups atomic.Int32

	config := Config{Output: "generated/assets"}

	for _, name := range []string{"world", "ui"} {
		action := pipelineFixtureAction("work")
		action.Parallel = true
		action.Run = func(ctx *Context) (Result, error) {
			active.Add(1)
			defer active.Add(-1)

			entered <- struct{}{}

			<-ctx.Done()

			return nil, ctx.Err()
		}
		config.Pipelines = append(config.Pipelines, pipelineCleanupBranch(name, action, &active, &cleanups))
	}

	controllerDone := make(chan struct{})
	go func() {
		defer close(controllerDone)

		for range jobs {
			if pipelineWait(ctx, entered) != nil {
				return
			}
		}

		cancel()
	}()

	project := t.TempDir()
	concurrentExecutionPublishOld(t, project)
	_, err := Build(ctx, config, Options{Project: project, Identity: "pipeline-cancel", Jobs: jobs})

	cancel()
	<-controllerDone

	if !errors.Is(err, context.Canceled) || errors.Is(err, errConcurrentCleanup) || cleanups.Load() != jobs {
		t.Fatalf("cancel/cleanup: %v; cleanups=%d", err, cleanups.Load())
	}

	assertConcurrentExecutionOld(t, project)
}

func TestPipelineConflictingPublicationPreservesOldTree(t *testing.T) {
	t.Parallel()

	project := t.TempDir()
	concurrentExecutionPublishOld(t, project)

	config := Config{Output: "generated/assets"}
	for _, name := range []string{"first", "second"} {
		config.Pipelines = append(config.Pipelines, Pipeline{Name: name, Output: []Plugin{fixturePlugin(func(plan *Plan) error {
			action := pipelineFixtureAction("publish")
			action.Parallel = true
			action.Run = func(*Context) (Result, error) {
				data, err := EncodeBundle(Bundle{"conflict.txt": []byte(name)})

				return Result{"data": data}, err
			}

			refs, err := plan.Add(action)
			if err == nil {
				plan.Publish(refs[0])
			}

			return err
		})}})
	}

	if _, err := Build(t.Context(), config, Options{Project: project, Identity: "pipeline-conflict", Jobs: 2}); err == nil {
		t.Fatal("conflicting publication succeeded")
	}

	assertConcurrentExecutionOld(t, project)
}

func TestPipelineSerialDependencyCannotBeBypassed(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(t.Context(), pipelineTestDeadline)
	defer cancel()

	probeFinished := make(chan struct{})

	var serialFinished atomic.Bool

	heavy := pipelineFixtureAction("work")
	heavy.Parallel = true
	heavy.Run = func(ctx *Context) (Result, error) {
		return Result{"data": nil}, pipelineWait(ctx, probeFinished)
	}
	dependent := Pipeline{Name: "dependent", Process: []Plugin{fixturePlugin(func(plan *Plan) error {
		first := pipelineFixtureAction("first")

		first.Parallel, first.Inputs = true, []Ref{{Action: "serial", Slot: "data"}}
		if _, err := plan.Add(first); err != nil {
			return err
		}

		second := pipelineFixtureAction("second")
		second.Parallel = true

		second.Run = func(*Context) (Result, error) {
			if !serialFinished.Load() {
				return nil, errFixtureFailure
			}

			return Result{"data": nil}, nil
		}
		if _, err := plan.Add(second); err != nil {
			return err
		}

		serial := pipelineFixtureAction("serial")
		serial.Run = func(*Context) (Result, error) {
			serialFinished.Store(true)

			return Result{"data": nil}, nil
		}
		_, err := plan.Add(serial)

		return err
	})}}
	probe := pipelineFixtureAction("work")
	probe.Parallel = true
	config := Config{Output: "generated/assets", Pipelines: []Pipeline{
		pipelineActionBranch("heavy", heavy), dependent, pipelineActionBranch("probe", probe),
	}}

	trace := func(event ActionEvent) {
		if event.Action == "pipeline/probe/work" && event.Phase == ActionFinished {
			close(probeFinished)
		}
	}
	if _, err := Build(ctx, config, Options{Project: t.TempDir(), Identity: "pipeline-dependency", Jobs: 2, Trace: trace}); err != nil {
		t.Fatal("a later parallel action bypassed a serial dependency", err)
	}
}

func TestPipelineDirectoriesSeparatePublicationPaths(t *testing.T) {
	t.Parallel()

	project := t.TempDir()

	config := Config{Output: "generated/assets"}
	for _, name := range []string{"ui", "sprites"} {
		config.Pipelines = append(
			config.Pipelines,
			Pipeline{Name: name, Directory: name, Output: []Plugin{fixturePlugin(func(plan *Plan) error {
				action := pipelineFixtureAction("publish")
				action.Run = func(*Context) (Result, error) {
					data, err := EncodeBundle(Bundle{"assets.go": []byte(name)})

					return Result{"data": data}, err
				}

				refs, err := plan.Add(action)
				if err == nil {
					plan.Publish(refs[0])
				}

				return err
			})}},
		)
	}

	if _, err := Build(t.Context(), config, Options{Project: project, Identity: "pipeline-directories", Jobs: 2}); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"ui", "sprites"} {
		if data, err := fixtureRead(project, "generated/assets/"+name+"/assets.go"); err != nil || string(data) != name {
			t.Fatalf("directory %s: %q %v", name, data, err)
		}
	}
}

func TestPipelineDirectorySyntaxRejectsBeforePluginWork(t *testing.T) {
	t.Parallel()

	for _, directory := range []string{".", "../outside", "/absolute", "a/../b", `a\b`, "a//b"} {
		t.Run(fmt.Sprintf("%q", directory), func(t *testing.T) {
			t.Parallel()

			project := t.TempDir()

			config := Config{Output: "generated/assets", Pipelines: []Pipeline{{Name: "confined", Directory: directory,
				Import: []Plugin{forbiddenStagePlugin{testing: t}}}}}
			if _, err := Build(t.Context(), config, Options{Project: project, Identity: "pipeline-directory"}); !errors.Is(err, errPath) {
				t.Fatalf("directory %q admitted: %v", directory, err)
			}

			entries, err := os.ReadDir(project)
			if err != nil || len(entries) != 0 {
				t.Fatalf("invalid directory changed project: %v %v", entries, err)
			}
		})
	}
}
