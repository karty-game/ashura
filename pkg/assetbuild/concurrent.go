//go:build !js

package assetbuild

import (
	"bytes"
	"context"
	"fmt"
	"maps"
	"time"
)

type concurrentOrder struct {
	run     *runner
	state   map[string]uint8
	actions []Action
}

func (run *runner) concurrentOrder(ctx context.Context, identities []string) ([]Action, error) {
	order := concurrentOrder{run: run, state: make(map[string]uint8)}
	for _, identity := range identities {
		if err := order.visit(ctx, identity); err != nil {
			return nil, err
		}
	}

	return order.actions, nil
}

func (order *concurrentOrder) visit(ctx context.Context, identity string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if order.run.state[identity] == actionDone || order.state[identity] == actionDone {
		return nil
	}

	if order.state[identity] == actionVisiting {
		return fmt.Errorf("cyclic asset action %q: %w", identity, errPlan)
	}

	action, found := order.run.actions[identity]
	if !found {
		return fmt.Errorf("missing asset action %q: %w", identity, errPlan)
	}

	order.state[identity] = actionVisiting
	for _, ref := range action.Inputs {
		if err := order.visit(ctx, ref.Action); err != nil {
			return err
		}

		if !actionHasSlot(order.run.actions[ref.Action], ref.Slot) {
			return fmt.Errorf("missing action/slot %s/%s: %w", ref.Action, ref.Slot, errPlan)
		}
	}

	order.state[identity] = actionDone
	order.actions = append(order.actions, action)

	return nil
}

func actionHasSlot(action Action, name string) bool {
	for _, slot := range action.Outputs {
		if slot.Name == name {
			return true
		}
	}

	return false
}

type actionCompletion struct {
	index     int
	outputs   []outputValue
	artifacts map[Artifact]bool
	report    Report
	log       []byte
	err       error
}

// Only concurrent workers clone graph metadata. Inline actions run with no
// overlapping workers, so they share maps while isolating their log and report.
// Payload bytes and completed provenance sets remain immutable in both cases.
func (run *runner) executionView(log *bytes.Buffer, concurrent bool) *runner {
	view := *run
	if concurrent {
		view.actions = maps.Clone(run.actions)
		view.outputs = maps.Clone(run.outputs)
		view.artifacts = maps.Clone(run.artifacts)
		view.state = maps.Clone(run.state)
	}

	view.report = Report{}
	view.log = log

	return &view
}

func executeConcurrentAction(ctx context.Context, index int, action Action, view *runner, log *bytes.Buffer) actionCompletion {
	err := view.process(ctx, action)

	result := actionCompletion{index: index, report: view.report, log: log.Bytes(), err: err}
	if err == nil {
		result.outputs = make([]outputValue, len(action.Outputs))
		for slotIndex, slot := range action.Outputs {
			result.outputs[slotIndex] = view.outputs[Ref{Action: action.ID, Slot: slot.Name}]
		}

		result.artifacts = view.artifacts[action.ID]
	}

	return result
}

func (run *runner) traceAction(action Action, phase ActionPhase, err error) {
	if run.trace != nil {
		run.trace(ActionEvent{Action: action.ID, Processor: action.Processor, Phase: phase, Time: time.Now(), Err: err})
	}
}
