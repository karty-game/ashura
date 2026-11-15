package lightmapper

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
)

// Statistics retains unbiased sample variances in linear response space.
// Pilot and production must use independent instances and sample scrambles.
//
//nolint:recvcheck // Accumulation mutates a pointer; immutable snapshot queries deliberately retain value semantics.
type Statistics struct {
	Count    uint64
	Mean, M2 [4][3]float64
}

var errStatistics = errors.New("lightmapper: invalid sample statistics or quality policy")

func (stats *Statistics) Add(value Moments) error {
	if stats == nil || stats.Count == math.MaxUint64 {
		return errStatistics
	}

	for _, response := range value {
		for _, channel := range response {
			if !receiverFinite(channel) || channel < 0 {
				return errStatistics
			}
		}
	}

	stats.Count++

	for kernel := range 4 {
		for channel := range 3 {
			x := float64(value[kernel][channel])
			delta := x - stats.Mean[kernel][channel]
			stats.Mean[kernel][channel] += delta / float64(stats.Count)
			stats.M2[kernel][channel] += delta * (x - stats.Mean[kernel][channel])
		}
	}

	return nil
}

// Merge combines disjoint fixed sample ranges without discarding their variance.
func (stats *Statistics) Merge(other Statistics) error {
	if stats == nil || math.MaxUint64-stats.Count < other.Count || !other.valid() || !stats.valid() {
		return errStatistics
	}

	if other.Count == 0 {
		return nil
	}

	if stats.Count == 0 {
		*stats = other

		return nil
	}

	total := stats.Count + other.Count

	for kernel := range 4 {
		for channel := range 3 {
			delta := other.Mean[kernel][channel] - stats.Mean[kernel][channel]
			stats.Mean[kernel][channel] += delta * float64(other.Count) / float64(total)
			stats.M2[kernel][channel] += other.M2[kernel][channel] + delta*delta*float64(stats.Count)*float64(other.Count)/float64(total)
		}
	}

	stats.Count = total

	return nil
}

// Moments returns the mean for downstream reconstruction; pilot means must not
// be mixed into the independently estimated production mean.
func (stats Statistics) Moments() (Moments, error) {
	var result Moments
	if stats.Count == 0 || !stats.valid() {
		return result, errStatistics
	}

	for kernel := range 4 {
		for channel := range 3 {
			value := float32(stats.Mean[kernel][channel])
			if !receiverFinite(value) {
				return Moments{}, errStatistics
			}

			result[kernel][channel] = value
		}
	}

	return result, nil
}

// QualityPolicy is an explicit offline accuracy request. Confidence scales the
// standard error diagnostic; it is not a guaranteed probabilistic error bound.
type QualityPolicy struct {
	Minimum, Maximum                         uint32
	AbsoluteError, RelativeError, Confidence float32
}

// Allocation is fixed before production starts. Identity includes the complete
// pilot and policy, preventing resume from silently selecting a different count.
type Allocation struct {
	Samples    uint32
	Capped     bool
	PilotCount uint64
	Identity   string
}

//nolint:musttag // The allocation identity binds the complete immutable statistics and policy records, not a public JSON schema.
func AllocateProduction(pilot Statistics, policy QualityPolicy) (Allocation, error) {
	if pilot.Count < 2 || !pilot.valid() || !qualityValid(policy) {
		return Allocation{}, errStatistics
	}

	required := float64(policy.Minimum)

	for kernel := range 4 {
		for channel := range 3 {
			variance := pilot.M2[kernel][channel] / float64(pilot.Count-1)
			tolerance := max(float64(policy.AbsoluteError), float64(policy.RelativeError)*pilot.Mean[kernel][channel])
			count := math.Ceil(float64(policy.Confidence) * float64(policy.Confidence) * variance / (tolerance * tolerance))
			required = max(required, count)
		}
	}

	encoded, err := json.Marshal(struct {
		Pilot  Statistics    `json:"pilot"`
		Policy QualityPolicy `json:"policy"`
	}{pilot, policy})
	if err != nil {
		return Allocation{}, err
	}

	digest := sha256.Sum256(encoded)

	return Allocation{
		Samples:    uint32(min(required, float64(policy.Maximum))),
		Capped:     required > float64(policy.Maximum),
		PilotCount: pilot.Count,
		Identity:   hex.EncodeToString(digest[:]),
	}, nil
}

// Converged assesses a completed fixed-count production pass. It never stops
// accumulation or feeds accepted production samples back into allocation.
func (stats Statistics) Converged(policy QualityPolicy) bool {
	if stats.Count < 2 || !stats.valid() || !qualityValid(policy) {
		return false
	}

	for kernel := range 4 {
		for channel := range 3 {
			standardError := math.Sqrt(stats.M2[kernel][channel] / float64(stats.Count-1) / float64(stats.Count))

			tolerance := max(float64(policy.AbsoluteError), float64(policy.RelativeError)*stats.Mean[kernel][channel])
			if standardError*float64(policy.Confidence) > tolerance {
				return false
			}
		}
	}

	return true
}

func (stats Statistics) valid() bool {
	for kernel := range 4 {
		for channel := range 3 {
			mean, m2 := stats.Mean[kernel][channel], stats.M2[kernel][channel]
			if math.IsNaN(mean) || math.IsInf(mean, 0) || math.IsNaN(m2) || math.IsInf(m2, 0) || mean < 0 || m2 < 0 {
				return false
			}
		}
	}

	return true
}

func qualityValid(policy QualityPolicy) bool {
	return policy.Minimum >= 2 && policy.Maximum >= policy.Minimum && receiverFinite(policy.AbsoluteError) &&
		receiverFinite(policy.RelativeError) &&
		receiverFinite(policy.Confidence) &&
		policy.AbsoluteError > 0 &&
		policy.RelativeError >= 0 &&
		policy.Confidence > 0
}
