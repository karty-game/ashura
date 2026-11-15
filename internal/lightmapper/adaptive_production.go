//go:build !js

package lightmapper

import (
	"context"
	"slices"
)

// runProduction checkpoints only complete independent fixed-count groups.
func (state *AdaptiveState) runProduction(
	ctx context.Context,
	runner AdaptiveRunner,
	scene *PreparedScene,
	receivers []Receiver,
	save func(AdaptiveState) error,
) error {
	options := state.Options

	var err error

	groups := make(map[uint32][]int)

	for i, allocation := range state.Allocation {
		if !state.Completed[i] {
			groups[allocation.Samples] = append(groups[allocation.Samples], i)
		}
	}

	counts := make([]uint32, 0, len(groups))
	for count := range groups {
		counts = append(counts, count)
	}

	slices.Sort(counts)

	for _, count := range counts {
		if err = ctx.Err(); err != nil {
			return err
		}

		indices := groups[count]

		batch := make([]Receiver, len(indices))
		for i, index := range indices {
			batch[i] = receivers[index]
		}

		productionOptions := options
		productionOptions.Samples = count
		productionOptions.Seed = state.ProductionSeed

		production, runErr := runner.Bake(ctx, scene, batch, productionOptions)
		if runErr != nil {
			return runErr
		}

		if len(production.Estimates) != len(indices) {
			return ErrInvalid
		}

		if err = state.commitProduction(receivers, indices, production.Estimates, count); err != nil {
			return err
		}

		if save != nil {
			if err = save(state.clone()); err != nil {
				return err
			}
		}
	}

	return nil
}

func (state *AdaptiveState) commitProduction(receivers []Receiver, indices []int, estimates []Estimate, count uint32) error {
	var err error

	for i, estimate := range estimates {
		index := indices[i]
		if estimate.Count != count || estimate.ReceiverID != receivers[index].ID {
			return ErrInvalid
		}

		if _, err = EstimateStatistics(estimate); err != nil {
			return err
		}

		state.Production[index] = estimate
		state.Completed[index] = true
	}

	return nil
}
