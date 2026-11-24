//go:build !js

package assetbuild

import (
	"context"
	"fmt"
)

type pipelineFrame struct {
	steps []planStep
	next  int
}

type pipelineCursor struct {
	plan           *Plan
	frames         []pipelineFrame
	order          []string
	ranks          map[string]int
	failure        error
	cutoff         int
	done           bool
	planningFailed bool
}

func (run *runner) executeConfiguredPlan(ctx context.Context, plan *Plan) error {
	scheduler := newPipelineScheduler(run, plan)

	return scheduler.execute(ctx)
}

// frontier admits one branch's current action region or performs cheap manifest
// discovery. It never waits for actions from another pipeline to finish.
func (scheduler *pipelineScheduler) frontier(ctx context.Context, cursor *pipelineCursor) ([]Action, error) {
	for len(cursor.frames) > 0 {
		frame := &cursor.frames[len(cursor.frames)-1]
		if frame.next == len(frame.steps) {
			cursor.frames = cursor.frames[:len(cursor.frames)-1]

			continue
		}

		step := frame.steps[frame.next]
		if step.expansion == nil && scheduler.run.state[step.action] == actionDone {
			frame.next++

			continue
		}

		goals := scheduler.run.pipelineGoals(*frame)

		order, err := scheduler.run.concurrentOrder(ctx, goals)
		if err != nil {
			return nil, err
		}

		for _, action := range order {
			if _, exists := cursor.ranks[action.ID]; !exists {
				cursor.ranks[action.ID] = len(cursor.order)
				cursor.order = append(cursor.order, action.ID)
			}
		}

		if len(order) != 0 {
			return order, nil
		}

		if step.expansion == nil {
			return nil, fmt.Errorf("pipeline has unfinished action %q: %w", step.action, errPlan)
		}

		if len(cursor.frames) > maxExpansionDepth {
			return nil, fmt.Errorf("asset expansion depth exceeds bound: %w", errPlan)
		}

		steps, err := scheduler.run.expandPlan(ctx, cursor.plan, *step.expansion, scheduler.plan.ids)
		if err != nil {
			return nil, fmt.Errorf("asset expansion %s: %w", step.expansion.ID, err)
		}

		frame.next++

		cursor.frames = append(cursor.frames, pipelineFrame{steps: steps})
	}

	cursor.done = true

	return nil, nil
}

func (run *runner) pipelineGoals(frame pipelineFrame) []string {
	step := frame.steps[frame.next]
	if step.expansion != nil {
		goals := make([]string, 0, len(step.expansion.Inputs))
		for _, ref := range step.expansion.Inputs {
			goals = append(goals, ref.Action)
		}

		return goals
	}

	goals := []string{step.action}
	if run.actions[step.action].Parallel {
		for _, next := range frame.steps[frame.next+1:] {
			if next.expansion != nil || !run.actions[next.action].Parallel {
				break
			}

			goals = append(goals, next.action)
		}
	}

	return goals
}
