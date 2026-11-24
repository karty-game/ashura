//go:build !js

package assetbuild

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
)

var errPlan = errors.New("invalid asset processing plan")

// Plan is built in plugin order; explicit references determine execution order.
type Plan struct {
	Project     string
	Output      string
	Input       []string
	Resources   []Resource
	Environment Environment
	namespace   string
	directory   string
	pipelines   []*Plan
	actions     []Action
	published   []Ref
	ids         map[string]bool
	steps       []planStep
	expanding   bool
	cleanups    []func() error
	inspection  *pipelineInspection
}

// Add registers an action and returns references in its declared output order.
func (plan *Plan) Add(action Action) ([]Ref, error) {
	if action.ID == "" || action.Processor == "" || action.Revision == "" || action.Run == nil || len(action.Outputs) == 0 {
		return nil, fmt.Errorf("action %q requires identity, processor, revision, outputs and implementation: %w", action.ID, errPlan)
	}

	action.ID = plan.scopedIdentity(action.ID)
	action.namespace = plan.namespace

	if plan.ids == nil {
		plan.ids = make(map[string]bool)
	}

	if plan.ids[action.ID] {
		return nil, fmt.Errorf("duplicate action %q: %w", action.ID, errPlan)
	}

	if len(plan.ids) >= maxPlanSteps {
		return nil, fmt.Errorf("asset graph exceeds action/expansion bound: %w", errPlan)
	}

	if action.Cache != CacheAutomatic && action.Cache != CacheDisabled {
		return nil, fmt.Errorf("action %q cache policy: %w", action.ID, errPlan)
	}

	if _, err := json.Marshal(action.Options); err != nil {
		return nil, fmt.Errorf("action %q options: %w", action.ID, err)
	}

	artifacts, err := canonicalArtifacts(action.Artifacts)
	if err != nil {
		return nil, err
	}

	refs := make([]Ref, 0, len(action.Outputs))

	names := make(map[string]bool)
	for _, slot := range action.Outputs {
		if slot.Name == "" || slot.Schema == "" || names[slot.Name] {
			return nil, fmt.Errorf("action %q duplicate/empty slot or schema %q: %w", action.ID, slot.Name, errPlan)
		}

		names[slot.Name] = true
		refs = append(refs, Ref{Action: action.ID, Slot: slot.Name})
	}

	action.Inputs = slices.Clone(action.Inputs)
	for index, ref := range action.Inputs {
		action.Inputs[index] = plan.scopedRef(ref)
	}

	action.Artifacts = artifacts
	action.Outputs = slices.Clone(action.Outputs)
	plan.ids[action.ID] = true
	plan.actions = append(plan.actions, action)
	plan.steps = append(plan.steps, planStep{action: action.ID})

	return refs, nil
}

// Publish registers a slot containing EncodeBundle bytes or a legacy JSON Bundle for final publication.
func (plan *Plan) Publish(ref Ref) {
	plan.published = append(plan.published, plan.scopedRef(ref))
}

// SlotSchema returns the declared immutable serialization schema of a reference.
func (plan *Plan) SlotSchema(ref Ref) (string, error) {
	ref = plan.scopedRef(ref)

	for _, action := range plan.actions {
		if action.ID != ref.Action {
			continue
		}

		for _, slot := range action.Outputs {
			if slot.Name == ref.Slot {
				return slot.Schema, nil
			}
		}
	}

	return "", fmt.Errorf("missing action/slot %s/%s: %w", ref.Action, ref.Slot, errPlan)
}

// Glob discovers sorted project-relative files without following symlinks.
// Discovery runs on every build; the resulting references drive action membership.
func (plan *Plan) Glob(pattern string) ([]string, error) {
	if plan.expanding {
		return nil, fmt.Errorf("expansion discovery must use declared manifests: %w", errPlan)
	}

	if plan.Output != "" {
		buildState, err := confined(plan.Project, stateDirectory)
		if err != nil {
			return nil, err
		}

		if err := validateSourceRoots(plan.Project, plan.Output, buildState, []string{pattern}); err != nil {
			return nil, err
		}
	}

	files, err := globFiles(plan.Project, pattern)
	if err == nil && plan.inspection != nil {
		plan.inspection.globs[pattern] = slices.Clone(files)
	}

	return files, err
}

// ReadFile reads an immutable planning input. Include derived values in action options.
// Expensive processing belongs in actions rather than planning.
func (plan *Plan) ReadFile(path string) ([]byte, error) {
	if plan.expanding {
		return nil, fmt.Errorf("expansion discovery must use declared manifests: %w", errPlan)
	}

	if plan.Output != "" {
		full, err := confined(plan.Project, path)
		if err != nil {
			return nil, err
		}

		buildState, err := confined(plan.Project, stateDirectory)
		if err != nil {
			return nil, err
		}

		if overlaps(full, plan.Output) || overlaps(full, buildState) {
			return nil, fmt.Errorf("planning input %q overlaps generated state: %w", path, errPath)
		}
	}

	return readProjectFile(plan.Project, path)
}
