//go:build !js

package assetbuild

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
)

const (
	maxPlanSteps      = 65536
	maxExpansionDepth = 32
)

type planStep struct {
	action    string
	expansion *Expansion
}

// Expand registers cheap manifest-driven discovery at this point in plugin order.
// Declared producers execute before discovery; discovered actions execute before
// subsequent plan steps. Nested discovery is bounded to 32 levels and the complete
// graph to 65,536 action/expansion identities. IDs share one namespace.
func (plan *Plan) Expand(expansion Expansion) error {
	expansion.ID = plan.scopedIdentity(expansion.ID)
	if expansion.ID == "" || expansion.Run == nil || plan.ids[expansion.ID] {
		return fmt.Errorf("invalid or duplicate expansion %q: %w", expansion.ID, errPlan)
	}

	if len(plan.ids) >= maxPlanSteps {
		return fmt.Errorf("asset graph exceeds action/expansion bound: %w", errPlan)
	}

	if plan.ids == nil {
		plan.ids = make(map[string]bool)
	}

	expansion.Inputs = slices.Clone(expansion.Inputs)
	for index := range expansion.Inputs {
		expansion.Inputs[index] = plan.scopedRef(expansion.Inputs[index])
	}

	plan.ids[expansion.ID] = true
	plan.steps = append(plan.steps, planStep{expansion: &expansion})

	return nil
}

// expandPlan commits discovery on the coordinator while unrelated pipelines
// continue. Returned child steps use the same scheduler as the original graph.
func (run *runner) expandPlan(ctx context.Context, plan *Plan, expansion Expansion, globalIDs map[string]bool) ([]planStep, error) {
	for _, ref := range expansion.Inputs {
		if _, found := run.outputs[ref]; !found {
			return nil, fmt.Errorf("expansion missing input %+v: %w", ref, errPlan)
		}
	}

	child := &Plan{Project: plan.Project, Output: plan.Output, Input: slices.Clone(plan.Input),
		Resources: slices.Clone(plan.Resources), Environment: plan.Environment,
		actions: slices.Clone(plan.actions), published: slices.Clone(plan.published),
		ids: maps.Clone(plan.ids), expanding: true, namespace: plan.namespace, directory: plan.directory}
	child.cleanups = slices.Clone(plan.cleanups)

	context := &Context{
		Context:   ctx,
		runner:    run,
		action:    Action{ID: expansion.ID, Inputs: expansion.Inputs, namespace: plan.namespace},
		expanding: true,
	}
	if err := expansion.Run(context, child); err != nil {
		return nil, errors.Join(err, closeCleanups(child.cleanups[len(plan.cleanups):]))
	}

	if context.err != nil {
		return nil, errors.Join(context.err, closeCleanups(child.cleanups[len(plan.cleanups):]))
	}

	if err := ctx.Err(); err != nil {
		return nil, errors.Join(err, closeCleanups(child.cleanups[len(plan.cleanups):]))
	}

	added := child.actions[len(plan.actions):]
	if err := validateExpansion(child, added); err != nil {
		return nil, errors.Join(err, closeCleanups(child.cleanups[len(plan.cleanups):]))
	}

	if len(globalIDs) > maxPlanSteps-(len(child.ids)-len(plan.ids)) {
		return nil, errors.Join(
			fmt.Errorf("asset graph exceeds action/expansion bound: %w", errPlan),
			closeCleanups(child.cleanups[len(plan.cleanups):]),
		)
	}

	for id := range child.ids {
		globalIDs[id] = true
	}

	plan.actions, plan.published, plan.ids, plan.Resources = child.actions, child.published, child.ids, child.Resources
	plan.cleanups = child.cleanups

	for _, action := range added {
		run.actions[action.ID] = action
	}

	return child.steps, nil
}

func closePlanCleanups(plan *Plan) error {
	var result error
	for _, pipeline := range slices.Backward(plan.pipelines) {
		result = errors.Join(result, closePlanCleanups(pipeline))
	}

	return errors.Join(result, closeCleanups(plan.cleanups))
}

func validateExpansion(plan *Plan, actions []Action) error {
	for _, action := range actions {
		for _, ref := range action.Inputs {
			if _, err := plan.SlotSchema(ref); err != nil {
				return err
			}
		}
	}

	for _, step := range plan.steps {
		if step.expansion == nil {
			continue
		}

		for _, ref := range step.expansion.Inputs {
			if _, err := plan.SlotSchema(ref); err != nil {
				return err
			}
		}
	}

	for _, ref := range plan.published {
		if _, err := plan.SlotSchema(ref); err != nil {
			return err
		}
	}

	return validateExpansionCycles(plan.actions, actions)
}

func (ctx *Context) discoveryError() error {
	ctx.err = fmt.Errorf("expansion discovery must use declared manifests and artifacts: %w", errPlan)

	return ctx.err
}

// Cleanup registers a build-scoped finalizer, called once in reverse registration
// order after success or failure. Lazy resources can register their Close closure
// before allocating. Cleanup errors join the build error. A failed expansion
// immediately closes only finalizers registered by its uncommitted child plan.
func (plan *Plan) Cleanup(cleanup func() error) {
	if cleanup != nil {
		plan.cleanups = append(plan.cleanups, cleanup)
	}
}

func closeCleanups(cleanups []func() error) error {
	var result error
	for _, cleanup := range slices.Backward(cleanups) {
		result = errors.Join(result, cleanup())
	}

	return result
}
