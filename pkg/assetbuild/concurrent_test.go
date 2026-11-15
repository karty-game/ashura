//go:build !js

package assetbuild

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

var (
	errConcurrentEarly    = errors.New("earlier logical failure")
	errConcurrentLate     = errors.New("later physical failure")
	errConcurrentCleanup  = errors.New("cleanup raced an active build resource")
	errConcurrentLifetime = errors.New("successful action canceled build helper context")
	errConcurrentInput    = errors.New("input snapshots differ")
	errConcurrentResource = errors.New("concurrent build resource users")
)

type concurrentFixture struct {
	calls                   *atomic.Int32
	gpuStarted, cpuObserved chan struct{}
	cpuToken, gpuToken      *ExecutionResource
	cpuActive, gpuActive    atomic.Int32
	workerDone              <-chan struct{}
	workerOpened            bool
	source                  []Ref
}

func concurrentExecutionConfig(calls *atomic.Int32) Config {
	fixture := &concurrentFixture{
		calls:       calls,
		gpuStarted:  make(chan struct{}),
		cpuObserved: make(chan struct{}),
		cpuToken:    NewExecutionResource(),
		gpuToken:    NewExecutionResource(),
	}

	return Config{Input: []string{"assets/**"}, Output: "generated/assets", Plugins: []Plugin{fixturePlugin(fixture.plan)}}
}

func (fixture *concurrentFixture) plan(plan *Plan) error {
	plan.Cleanup(fixture.cleanup)

	source, err := plan.Add(
		Action{
			ID:        "source",
			Processor: "source",
			Revision:  "1",
			Outputs:   []Slot{{Name: "data", Schema: "data@1"}},
			Run:       fixture.prepareSource,
		},
	)
	if err != nil {
		return err
	}

	fixture.source = source

	cpu, err := fixture.add(plan, "cpu", fixture.cpuToken, fixture.cpu)
	if err != nil {
		return err
	}

	if _, err = fixture.add(plan, "gpu-first", fixture.gpuToken, fixture.gpuFirst); err != nil {
		return err
	}

	second, err := fixture.add(plan, "gpu-second", fixture.gpuToken, fixture.gpuSecond)
	if err != nil {
		return err
	}

	final, err := plan.Add(
		Action{
			ID:        "final",
			Processor: "final",
			Revision:  "1",
			Inputs:    append(append([]Ref(nil), cpu...), second...),
			Outputs:   []Slot{{Name: "files", Schema: "files@1"}},
			Run: func(ctx *Context) (Result, error) {
				fixture.calls.Add(1)

				data, err := ctx.Input(second[0])
				if err != nil {
					return nil, err
				}

				encoded, err := EncodeBundle(Bundle{"value.bin": data})

				return Result{"files": encoded}, err
			},
		},
	)
	if err == nil {
		plan.Publish(final[0])
	}

	return err
}

func (fixture *concurrentFixture) add(
	plan *Plan,
	identity string,
	token *ExecutionResource,
	callback func(*Context) (Result, error),
) ([]Ref, error) {
	return plan.Add(
		Action{
			ID:        identity,
			Processor: identity,
			Revision:  "1",
			Parallel:  true,
			Exclusive: token,
			Inputs:    fixture.source,
			Outputs:   []Slot{{Name: "data", Schema: "data@1"}},
			Run:       callback,
		},
	)
}

func (fixture *concurrentFixture) alive() bool {
	if !fixture.workerOpened {
		return false
	}

	select {
	case <-fixture.workerDone:
		return false
	default:
		return true
	}
}

func (fixture *concurrentFixture) cleanup() error {
	if fixture.cpuActive.Load() != 0 || fixture.gpuActive.Load() != 0 {
		return errConcurrentCleanup
	}

	if fixture.workerOpened && !fixture.alive() {
		return errConcurrentLifetime
	}

	return nil
}

func (fixture *concurrentFixture) prepareSource(ctx *Context) (Result, error) {
	fixture.calls.Add(1)

	data, err := ctx.ReadFile("assets/source.txt")
	if err != nil {
		return nil, err
	}

	artifact, err := ctx.StoreArtifact(data)
	if err != nil {
		return nil, err
	}

	encoded, err := json.Marshal(artifact)

	return Result{"data": encoded}, err
}

func (fixture *concurrentFixture) read(ctx *Context) (Result, error) {
	fixture.calls.Add(1)

	encoded, err := ctx.Input(fixture.source[0])
	if err != nil {
		return nil, err
	}

	var artifact Artifact
	if err = json.Unmarshal(encoded, &artifact); err != nil {
		return nil, err
	}

	data, err := ctx.ReadArtifact(artifact)
	if err != nil {
		return nil, err
	}

	snapshot, err := ctx.ReadFile("assets/source.txt")
	if err != nil || string(snapshot) != string(data) {
		return nil, errConcurrentInput
	}

	return Result{"data": data}, nil
}

func (fixture *concurrentFixture) cpu(ctx *Context) (Result, error) {
	if fixture.cpuActive.Add(1) != 1 {
		return nil, errConcurrentResource
	}

	defer fixture.cpuActive.Add(-1)

	select {
	case <-fixture.gpuStarted:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	close(fixture.cpuObserved)

	return fixture.read(ctx)
}

func (fixture *concurrentFixture) gpuFirst(ctx *Context) (Result, error) {
	if fixture.gpuActive.Add(1) != 1 {
		return nil, errConcurrentResource
	}

	defer fixture.gpuActive.Add(-1)

	fixture.workerDone = ctx.Done()
	fixture.workerOpened = true
	close(fixture.gpuStarted)

	select {
	case <-fixture.cpuObserved:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	return fixture.read(ctx)
}

func (fixture *concurrentFixture) gpuSecond(ctx *Context) (Result, error) {
	if fixture.gpuActive.Add(1) != 1 {
		return nil, errConcurrentResource
	}

	defer fixture.gpuActive.Add(-1)

	if !fixture.alive() {
		return nil, errConcurrentLifetime
	}

	return fixture.read(ctx)
}

func TestConcurrentExecutionOverlapTokensCacheAndCleanup(t *testing.T) {
	t.Parallel()
	project := t.TempDir()
	fixtureWrite(t, project, "assets/source.txt", []byte("first"))

	var calls atomic.Int32

	options := Options{Project: project, Identity: "concurrent-fixture", Jobs: 2}

	for iteration := range 3 {
		options.Jobs = 2
		if iteration == 1 {
			options.Jobs = 1
		}

		before := calls.Load()

		report, err := Build(t.Context(), concurrentExecutionConfig(&calls), options)
		if err != nil {
			t.Fatal(err)
		}

		if iteration == 1 {
			if calls.Load() != before || report.Reused != 5 {
				t.Fatal("cache hit executed actions", calls.Load(), report)
			}
		} else if report.Executed != 5 || calls.Load()-before != 5 {
			t.Fatal("missing action", calls.Load(), report)
		}

		//nolint:gosec // G304: fixed fixture path below this test's private TempDir.
		data, err := os.ReadFile(filepath.Join(project, "generated/assets/value.bin"))
		if err != nil {
			t.Fatal(err)
		}

		expected := "first"
		if iteration == 2 {
			expected = "changed"
		}

		if string(data) != expected {
			t.Fatal("wrong publication", string(data))
		}

		if iteration == 1 {
			fixtureWrite(t, project, "assets/source.txt", []byte("changed"))
		}
	}
}

func concurrentExecutionPublishOld(t *testing.T, project string) {
	t.Helper()

	config := Config{Output: "generated/assets", Plugins: []Plugin{fixturePlugin(func(plan *Plan) error {
		refs, err := plan.Add(
			Action{
				ID:        "old",
				Processor: "old",
				Revision:  "1",
				Outputs:   []Slot{{Name: "files", Schema: "files@1"}},
				Run: func(*Context) (Result, error) {
					data, err := EncodeBundle(Bundle{"value.bin": []byte("old")})

					return Result{"files": data}, err
				},
			},
		)
		if err == nil {
			plan.Publish(refs[0])
		}

		return err
	})}}
	if _, err := Build(t.Context(), config, Options{Project: project, Identity: "old"}); err != nil {
		t.Fatal(err)
	}
}

func assertConcurrentExecutionOld(t *testing.T, project string) {
	t.Helper()

	//nolint:gosec // G304: fixed fixture path below this test's private TempDir.
	data, err := os.ReadFile(filepath.Join(project, "generated/assets/value.bin"))
	if err != nil || string(data) != "old" {
		t.Fatal("failed build changed publication", string(data), err)
	}
}
