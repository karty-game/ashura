//go:build !js

package lightmapper

func (state *AdaptiveState) assess(receivers []Receiver) error {
	state.Unconverged = nil
	for receiverIndex, estimate := range state.Production {
		stats, statsErr := EstimateStatistics(estimate)
		if statsErr != nil {
			return statsErr
		}

		if state.Allocation[receiverIndex].Capped || !stats.Converged(state.Policy.Quality) {
			state.Unconverged = append(state.Unconverged, receivers[receiverIndex].ID)
		}
	}

	return nil
}
