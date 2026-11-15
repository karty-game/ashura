//go:build !js

package assetbuild

import (
	"bytes"
	"fmt"
	"testing"
)

//nolint:paralleltest // AllocsPerRun changes process-wide GOMAXPROCS on Go 1.27.
func TestSerialExecutionViewAllocationDoesNotScaleWithGraph(t *testing.T) {
	for _, size := range []int{1, 4096} {
		run := executionAllocationFixture(size)

		var (
			log  bytes.Buffer
			view *runner
		)

		allocations := testing.AllocsPerRun(10, func() {
			view = run.executionView(&log, false)
		})
		// One fixed runner snapshot may escape; graph size must add no allocation.
		if allocations > 1 {
			t.Fatalf("serial view copied graph metadata: size=%d allocations=%g", size, allocations)
		}

		if len(view.actions) != size || len(view.outputs) != size || len(view.artifacts) != size || len(view.state) != size {
			t.Fatal("allocation guard did not retain the complete graph")
		}
	}
}

func executionAllocationFixture(size int) *runner {
	run := &runner{
		actions: make(map[string]Action, size), outputs: make(map[Ref]outputValue, size),
		artifacts: make(map[string]map[Artifact]bool, size), state: make(map[string]uint8, size),
	}
	for index := range size {
		id := fmt.Sprintf("action/%d", index)
		run.actions[id] = Action{ID: id}
		run.outputs[Ref{Action: id, Slot: "data"}] = outputValue{}
		run.artifacts[id] = nil
		run.state[id] = actionDone
	}

	return run
}
