//go:build !js

package lightmapper

import "testing"

func TestStableVarianceDoesNotSubtractLargeMeanSquared(t *testing.T) {
	t.Parallel()

	estimate := Estimate{Count: 32, VarianceValid: true, SecondMomentsValid: false}
	for kernel := range 4 {
		estimate.Statistics[kernel] = Vec3{1e20, 1e20, 1e20}
		estimate.Variance[kernel] = Vec3{.25, .25, .25}
	}

	stats, err := EstimateStatistics(estimate)
	if err != nil || stats.M2[0][0] != .25*31 {
		t.Fatal(stats, err)
	}

	policy := QualityPolicy{Minimum: 2, Maximum: 4096, AbsoluteError: .01, RelativeError: 0, Confidence: 2}

	allocation, err := AllocateProduction(stats, policy)
	if err != nil || !allocation.Capped {
		t.Fatal(allocation, err)
	}
}
