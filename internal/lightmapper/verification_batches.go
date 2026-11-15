//go:build !js

package lightmapper

import (
	"context"
	"errors"
)

// VerificationBatch keeps one independently selected receiver domain, policy and
// report. Combining execution never combines its identity, derived seed or coverage.
type VerificationBatch struct {
	Receivers []Receiver
	Estimates []Estimate
	Options   Options
	Policy    VerificationPolicy
}

type verificationBatchGroup struct {
	request    verificationGPURequest
	pages      []int
	ends       []int
	identities map[uint64]bool
	intervals  []verificationGPUInterval
}

// VerifyRNMBatches independently prepares every report before dispatch, then
// combines compatible reference passes with explicit per-receiver seeds. Reports
// retain input order and match separate VerifyRNM calls. ErrVerification returns
// all completed reports; invalid input or execution failure never encodes output.
func (worker *GPUWorker) VerifyRNMBatches(
	ctx context.Context,
	scene *PreparedScene,
	batches []VerificationBatch,
) ([]VerificationReport, error) {
	if worker == nil || ctx == nil || scene == nil || len(batches) == 0 {
		return nil, ErrInvalid
	}

	state, err := worker.prepareVerificationBatches(ctx, scene, batches)
	if err != nil {
		return state.reports, err
	}

	if len(state.groups) == 0 {
		return state.reports, state.qualityError
	}

	packed, err := worker.scenePacket(scene)
	if err != nil {
		return state.reports, err
	}

	// Admit every group and memory split before the first executor call.

	for index := range state.groups {
		group := &state.groups[index]

		group.intervals, err = worker.verificationIntervals(ctx, scene, packed, group.request)
		if err != nil {
			return state.reports, err
		}
	}

	for _, group := range state.groups {
		responses, runErr := worker.runVerificationIntervals(ctx, scene, packed, group.request, group.intervals)
		if runErr != nil {
			return state.reports, runErr
		}

		if err = state.finishGroup(group, responses, batches); err != nil {
			return state.reports, err
		}
	}

	return state.reports, state.qualityError
}

type verificationBatchExecution struct {
	plans        []verificationPlan
	reports      []VerificationReport
	groups       []verificationBatchGroup
	qualityError error
}

func (worker *GPUWorker) prepareVerificationBatches(
	ctx context.Context,
	scene *PreparedScene,
	batches []VerificationBatch,
) (verificationBatchExecution, error) {
	state := verificationBatchExecution{
		plans:   make([]verificationPlan, len(batches)),
		reports: make([]VerificationReport, len(batches)),
		groups:  make([]verificationBatchGroup, 0, len(batches)),
	}

	for index, batch := range batches {
		if err := ctx.Err(); err != nil {
			return state, err
		}

		plan, err := prepareVerification(scene, batch.Receivers, batch.Estimates, batch.Options, batch.Policy, worker.Device().Adapter)
		state.reports[index] = plan.Report

		if errors.Is(err, ErrVerification) {
			state.qualityError = ErrVerification

			continue
		}

		if err != nil {
			return state, err
		}

		effective, err := normalizeOptions(plan.Options)
		if err != nil {
			return state, err
		}

		state.plans[index] = plan

		if err = appendVerificationGroup(&state.groups, index, plan, effective); err != nil {
			return state, err
		}
	}

	return state, nil
}

func (state *verificationBatchExecution) finishGroup(
	group verificationBatchGroup,
	responses []ResponseEstimate,
	batches []VerificationBatch,
) error {
	first := 0

	for index, page := range group.pages {
		end := group.ends[index]
		report, err := finishVerification(state.plans[page], responses[first:end], batches[page].Policy)
		state.reports[page] = report

		if err != nil {
			if !errors.Is(err, ErrVerification) {
				return err
			}

			state.qualityError = ErrVerification
		}

		first = end
	}

	return nil
}

func appendVerificationGroup(groups *[]verificationBatchGroup, page int, plan verificationPlan, effective Options) error {
	if len(plan.Receivers) > 1<<20 {
		return ErrInvalid
	}

	identities := make(map[uint64]bool, len(plan.Receivers))

	for _, receiver := range plan.Receivers {
		if identities[receiver.ID] {
			return ErrInvalid
		}

		identities[receiver.ID] = true
	}

	// The packet's legacy job seed is ignored only where an explicit seed exists.
	// Keep zero here for compatibility comparison; every page seed is stored below.
	effective.Seed = 0
	selected := -1

	for index := range *groups {
		group := &(*groups)[index]

		if group.request.options == effective && len(group.request.receivers)+len(plan.Receivers) <= 1<<20 &&
			!verificationIdentityOverlap(group.identities, identities) {
			selected = index

			break
		}
	}

	if selected < 0 {
		*groups = append(
			*groups,
			verificationBatchGroup{request: verificationGPURequest{options: effective}, identities: make(map[uint64]bool)},
		)
		selected = len(*groups) - 1
	}

	group := &(*groups)[selected]
	group.request.receivers = append(group.request.receivers, plan.Receivers...)
	group.request.normals = append(group.request.normals, plan.Normals...)

	for _, receiver := range plan.Receivers {
		group.request.seeds = append(group.request.seeds, plan.Options.Seed)
		group.identities[receiver.ID] = true
	}

	group.pages = append(group.pages, page)
	group.ends = append(group.ends, len(group.request.receivers))

	return nil
}

func verificationIdentityOverlap(first, second map[uint64]bool) bool {
	for id := range second {
		if first[id] {
			return true
		}
	}

	return false
}
