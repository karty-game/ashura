package lightmapper

import (
	"math"
	"testing"
)

func statisticsFixture(t *testing.T, first, last int) Statistics {
	t.Helper()

	var result Statistics

	for sample := first; sample < last; sample++ {
		var value Moments

		for kernel := range 4 {
			for channel := range 3 {
				value[kernel][channel] = float32(sample*sample + kernel + channel)
			}
		}

		if err := result.Add(value); err != nil {
			t.Fatal(err)
		}
	}

	return result
}

func TestDisjointStatisticsMergeAndFrozenAllocation(t *testing.T) {
	t.Parallel()
	full := statisticsFixture(t, 0, 32)

	merged := statisticsFixture(t, 0, 8)
	if err := merged.Merge(statisticsFixture(t, 8, 32)); err != nil {
		t.Fatal(err)
	}

	for kernel := range 4 {
		for channel := range 3 {
			if math.Abs(full.Mean[kernel][channel]-merged.Mean[kernel][channel]) > 1e-8 ||
				math.Abs(full.M2[kernel][channel]-merged.M2[kernel][channel]) > 1e-6 {
				t.Fatal(full, merged)
			}
		}
	}

	policy := QualityPolicy{Minimum: 8, Maximum: 4096, AbsoluteError: .01, RelativeError: .01, Confidence: 2}

	first, err := AllocateProduction(full, policy)
	if err != nil {
		t.Fatal(err)
	}

	second, err := AllocateProduction(full, policy)
	if err != nil || first != second || first.Identity == "" || first.Samples < 8 {
		t.Fatal(first, second, err)
	}
}

func TestCheckpointRejectsOverlapAndDifferentAttempt(t *testing.T) {
	t.Parallel()

	identity := CheckpointIdentity{
		Scene:      "scene",
		Receivers:  "receivers",
		Algorithm:  "algorithm",
		Backend:    "backend",
		Pilot:      "pilot",
		Allocation: "allocation",
		Production: "production",
	}
	checkpoint := Checkpoint{
		Schema:   CheckpointSchema,
		Identity: identity,
		Ranges:   []CommittedRange{{ReceiverID: 1, First: 0, Count: 8, Statistics: statisticsFixture(t, 0, 8)}},
	}

	encoded, err := checkpoint.Encode()
	if err != nil {
		t.Fatal(err)
	}

	if _, err = DecodeCheckpoint(encoded, identity); err != nil {
		t.Fatal(err)
	}

	other := identity

	other.Production = "another-attempt"
	if _, err = DecodeCheckpoint(encoded, other); err == nil {
		t.Fatal("accepted different production scramble")
	}

	checkpoint.Ranges = append(checkpoint.Ranges, CommittedRange{ReceiverID: 1, First: 7, Count: 2, Statistics: statisticsFixture(t, 7, 9)})
	if err = checkpoint.Validate(identity); err == nil {
		t.Fatal("accepted overlapping range")
	}
}
