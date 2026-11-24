//go:build !js

package assetbuild

import (
	"fmt"
	"slices"
	"strings"
)

func (plan *Plan) scopedIdentity(id string) string {
	if plan.namespace == "" || id == "" || strings.HasPrefix(id, plan.namespace) {
		return id
	}

	return plan.namespace + id
}

func (plan *Plan) scopedRef(ref Ref) Ref {
	ref.Action = plan.scopedIdentity(ref.Action)

	return ref
}

func planPipelines(root *Plan, definitions []pipelineDefinition, inspect bool) error {
	for _, definition := range definitions {
		pipeline := definition.pipeline
		plan := &Plan{Project: root.Project, Output: root.Output, Input: slices.Clone(pipeline.Input),
			Environment: root.Environment, namespace: definition.namespace, directory: pipeline.Directory}

		if inspect {
			plan.inspection = newPipelineInspection(pipeline.Name)
		}

		// Retain cleanup ownership even when a later plugin fails to plan.
		root.pipelines = append(root.pipelines, plan)

		for _, stage := range [3][]Plugin{pipeline.Import, pipeline.Process, pipeline.Output} {
			for _, plugin := range stage {
				if plugin == nil || plugin.Name() == "" {
					return fmt.Errorf("pipeline %q has nil/unnamed asset plugin: %w", pipeline.Name, errPlan)
				}

				if err := plugin.Plan(plan); err != nil {
					return fmt.Errorf("pipeline %q asset plugin %s: %w", pipeline.Name, plugin.Name(), err)
				}
			}
		}

		if err := mergePipelineActions(root, plan); err != nil {
			return err
		}
	}

	return nil
}

func mergePipelineActions(root, plan *Plan) error {
	if len(root.ids) > maxPlanSteps-len(plan.ids) {
		return fmt.Errorf("asset graph exceeds action/expansion bound: %w", errPlan)
	}

	if root.ids == nil {
		root.ids = make(map[string]bool)
	}

	for id := range plan.ids {
		if root.ids[id] {
			return fmt.Errorf("duplicate pipeline action/expansion %q: %w", id, errPlan)
		}

		root.ids[id] = true
	}

	root.actions = append(root.actions, plan.actions...)

	return nil
}
