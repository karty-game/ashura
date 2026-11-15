//go:build !js

package assetbuild

import (
	"bytes"
	"context"
	"fmt"
)

type pipelineCompletion struct {
	cursor *pipelineCursor
	action Action
	result actionCompletion
}

type pipelineScheduler struct {
	run                  *runner
	plan                 *Plan
	cursors              []*pipelineCursor
	completed            chan pipelineCompletion
	started              map[string]bool
	results              map[string]*actionCompletion
	busy                 map[*ExecutionResource]bool
	limit, running, next int
	serial               bool
}

func newPipelineScheduler(run *runner, plan *Plan) *pipelineScheduler {
	limit := max(1, run.jobs)

	scheduler := &pipelineScheduler{
		run: run, plan: plan, limit: limit,
		completed: make(chan pipelineCompletion, limit),
		started:   make(map[string]bool), results: make(map[string]*actionCompletion),
		busy: make(map[*ExecutionResource]bool),
	}
	for _, branch := range plan.pipelines {
		scheduler.cursors = append(scheduler.cursors, &pipelineCursor{
			plan: branch, frames: []pipelineFrame{{steps: branch.steps}}, ranks: make(map[string]int), cutoff: maxPlanSteps,
		})
	}

	return scheduler
}

func (scheduler *pipelineScheduler) execute(ctx context.Context) error {
	for {
		scheduler.dispatch(ctx)

		if scheduler.running == 0 {
			break
		}

		scheduler.accept(<-scheduler.completed)
	}

	scheduler.flush()

	if err := ctx.Err(); err != nil {
		return err
	}

	for _, cursor := range scheduler.cursors {
		if cursor.failure != nil {
			return cursor.failure
		}

		if !cursor.done {
			return fmt.Errorf("pipeline ready graph stalled: %w", errPlan)
		}
	}

	return nil
}

func (scheduler *pipelineScheduler) dispatch(ctx context.Context) {
	for ctx.Err() == nil && scheduler.running < scheduler.limit && !scheduler.serial {
		if !scheduler.dispatchNext(ctx) {
			return
		}
	}
}

func (scheduler *pipelineScheduler) dispatchNext(ctx context.Context) bool {
	progress := false

	last := len(scheduler.cursors) - 1
	for index, cursor := range scheduler.cursors {
		if cursor.failure != nil {
			last = index

			break
		}
	}

	for offset := range len(scheduler.cursors) {
		index := (scheduler.next + offset) % len(scheduler.cursors)
		if index > last {
			continue
		}

		cursor := scheduler.cursors[index]
		if cursor.done || cursor.planningFailed {
			continue
		}

		if scheduler.limit == 1 && !scheduler.precedingDone(index) {
			continue
		}

		actions, err := scheduler.frontier(ctx, cursor)
		if err != nil {
			cursor.failure, cursor.cutoff = err, len(cursor.order)
			cursor.planningFailed = true

			return true
		}

		if cursor.done {
			progress = true
		}

		for _, action := range actions {
			if !scheduler.ready(cursor, action) {
				if !action.Parallel {
					break
				}

				continue
			}

			scheduler.launch(ctx, cursor, action)
			scheduler.next = (index + 1) % len(scheduler.cursors)

			return true
		}
	}

	return progress
}

func (scheduler *pipelineScheduler) precedingDone(index int) bool {
	for _, cursor := range scheduler.cursors[:index] {
		if !cursor.done {
			return false
		}
	}

	return true
}

func (scheduler *pipelineScheduler) ready(cursor *pipelineCursor, action Action) bool {
	if scheduler.started[action.ID] || cursor.ranks[action.ID] >= cursor.cutoff || !action.Parallel && scheduler.running != 0 {
		return false
	}

	if action.Exclusive != nil && scheduler.busy[action.Exclusive] {
		return false
	}

	for _, ref := range action.Inputs {
		if scheduler.run.state[ref.Action] != actionDone {
			return false
		}
	}

	return true
}

func (scheduler *pipelineScheduler) launch(ctx context.Context, cursor *pipelineCursor, action Action) {
	var log bytes.Buffer

	concurrent := scheduler.limit > 1 && action.Parallel
	view := scheduler.run.executionView(&log, concurrent)
	scheduler.started[action.ID] = true
	scheduler.running++

	scheduler.serial = !action.Parallel
	if action.Exclusive != nil {
		scheduler.busy[action.Exclusive] = true
	}

	scheduler.run.traceAction(action, ActionStarted, nil)

	rank := cursor.ranks[action.ID]
	if !concurrent {
		result := executeConcurrentAction(ctx, rank, action, view, &log)
		scheduler.completed <- pipelineCompletion{cursor: cursor, action: action, result: result}

		return
	}

	go func() {
		result := executeConcurrentAction(ctx, rank, action, view, &log)
		scheduler.completed <- pipelineCompletion{cursor: cursor, action: action, result: result}
	}()
}

func (scheduler *pipelineScheduler) accept(completion pipelineCompletion) {
	scheduler.running--
	if !completion.action.Parallel {
		scheduler.serial = false
	}

	if completion.action.Exclusive != nil {
		delete(scheduler.busy, completion.action.Exclusive)
	}

	result := completion.result
	scheduler.run.traceAction(completion.action, ActionFinished, result.err)

	if result.err != nil {
		if result.index < completion.cursor.cutoff {
			completion.cursor.cutoff = result.index
			completion.cursor.failure = fmt.Errorf(
				"asset action %s (%s): %w",
				completion.action.ID,
				completion.action.Processor,
				result.err,
			)
		}
	} else {
		for index, slot := range completion.action.Outputs {
			scheduler.run.outputs[Ref{Action: completion.action.ID, Slot: slot.Name}] = result.outputs[index]
		}

		scheduler.run.artifacts[completion.action.ID] = result.artifacts
		scheduler.run.state[completion.action.ID] = actionDone
	}

	result.outputs, result.artifacts = nil, nil
	scheduler.results[completion.action.ID] = &result
}

func (scheduler *pipelineScheduler) flush() {
	for _, cursor := range scheduler.cursors {
		for _, id := range cursor.order {
			result := scheduler.results[id]
			if result == nil {
				return
			}

			if scheduler.run.log != nil {
				_, _ = scheduler.run.log.Write(result.log)
			}

			if result.err != nil {
				return
			}

			scheduler.run.report.Executed += result.report.Executed
			scheduler.run.report.Reused += result.report.Reused
			scheduler.run.report.Actions = append(scheduler.run.report.Actions, result.report.Actions...)
		}

		if cursor.failure != nil {
			return
		}
	}
}
