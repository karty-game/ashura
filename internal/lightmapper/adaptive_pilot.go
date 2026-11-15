//go:build !js

package lightmapper

import "context"

// preparePilot freezes every allocation before any production sample is used.
func (state *AdaptiveState) preparePilot(
	ctx context.Context,
	runner AdaptiveRunner,
	scene *PreparedScene,
	receivers []Receiver,
	save func(AdaptiveState) error,
) error {
	options, policy := state.Options, state.Policy

	var err error

	pilotOptions := options
	pilotOptions.Samples = policy.PilotSamples
	pilotOptions.Seed = state.PilotSeed

	pilot, runErr := runner.Bake(ctx, scene, receivers, pilotOptions)
	if runErr != nil {
		return runErr
	}

	if len(pilot.Estimates) != len(receivers) {
		return ErrInvalid
	}

	state.Pilot = pilot.Estimates
	state.Allocation = make([]Allocation, len(receivers))
	state.Completed = make([]bool, len(receivers))

	state.Production = make([]Estimate, len(receivers))
	for receiverIndex, estimate := range pilot.Estimates {
		if estimate.ReceiverID != receivers[receiverIndex].ID || estimate.Count != policy.PilotSamples {
			return ErrInvalid
		}

		stats, statsErr := EstimateStatistics(estimate)
		if statsErr != nil {
			return statsErr
		}

		state.Allocation[receiverIndex], err = AllocateProduction(stats, policy.Quality)
		if err != nil {
			return err
		}
	}

	if save != nil {
		if err = save(state.clone()); err != nil {
			return err
		}
	}

	return nil
}
