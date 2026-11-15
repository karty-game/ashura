//go:build !js

package lightmapper

import (
	"encoding/binary"
	"math"
)

// DecodeGPUEstimates admits complete linear compute results before publication.
// The 56-word layout includes projected coefficients and stable variance; no CPU fitting substitutes
// for the hardware resolve output.
func DecodeGPUEstimates(data []byte, receivers []Receiver) ([]Estimate, error) {
	if uint64(len(receivers))*gpuResultWords*4 != uint64(len(data)) {
		return nil, ErrInvalid
	}

	result := make([]Estimate, len(receivers))
	if err := decodeGPUEstimatesInto(data, receivers, result); err != nil {
		return nil, err
	}

	return result, nil
}

// Private batch targets are unpublished until the complete roster succeeds.
// This retains every record check without allocating and copying another result.
//
//nolint:mnd // Word offsets and flag bits retain the fixed 56-word GPU result ABI.
func decodeGPUEstimatesInto(data []byte, receivers []Receiver, result []Estimate) error {
	if !gpuEstimateTargetFits(data, receivers, result) {
		return ErrInvalid
	}

	for index, receiver := range receivers {
		at := index * gpuResultWords * 4

		words := make([]uint32, gpuResultWords)
		for i := range gpuResultWords {
			words[i] = binary.LittleEndian.Uint32(data[at+i*4:])
		}

		vector := func(offset int) Vec3 {
			return Vec3{math.Float32frombits(words[offset]), math.Float32frombits(words[offset+1]), math.Float32frombits(words[offset+2])}
		}

		value := Estimate{ReceiverID: receiver.ID, Count: words[33], Direct: vector(24), Environment: vector(27), Reflected: vector(30)}
		if value.Count == 0 || (value.Count > 1 && words[55]&1 == 0) || words[55]&^uint32(3) != 0 {
			return ErrInvalid
		}

		value.VarianceValid = words[55]&1 != 0

		value.SecondMomentsValid = words[55]&2 != 0
		for kernel := range 4 {
			value.Statistics[kernel] = vector(kernel * 3)
			value.SecondMoments[kernel] = vector(12 + kernel*3)

			value.Variance[kernel] = vector(43 + kernel*3)
			if !positiveVec(value.Statistics[kernel]) || !positiveVec(value.Variance[kernel]) ||
				(!value.VarianceValid && value.Variance[kernel] != (Vec3{})) ||
				(value.SecondMomentsValid && !positiveVec(value.SecondMoments[kernel])) {
				return ErrInvalid
			}
		}

		for basis := range 3 {
			value.RNM[basis] = vector(34 + basis*3)
			if !positiveVec(value.RNM[basis]) {
				return ErrInvalid
			}
		}

		if !positiveVec(value.Direct) || !positiveVec(value.Environment) || !positiveVec(value.Reflected) {
			return ErrInvalid
		}

		result[index] = value
	}

	return nil
}

func gpuEstimateTargetFits(data []byte, receivers []Receiver, result []Estimate) bool {
	return len(receivers) == len(result) && uint64(len(receivers))*gpuResultWords*gpuWordBytes == uint64(len(data))
}

// EstimateStatistics consumes authoritative stable sample variance. Raw second
// moments are optional diagnostics and are never subtracted for quality decisions.
func EstimateStatistics(value Estimate) (Statistics, error) {
	if value.Count == 0 || !value.VarianceValid {
		return Statistics{}, errStatistics
	}

	result := Statistics{Count: uint64(value.Count)}

	for kernel := range 4 {
		for channel := range 3 {
			mean, variance := value.Statistics[kernel][channel], value.Variance[kernel][channel]
			if !receiverFinite(mean) || !receiverFinite(variance) || mean < 0 || variance < 0 {
				return Statistics{}, errStatistics
			}

			result.Mean[kernel][channel] = float64(mean)
			if value.Count > 1 {
				result.M2[kernel][channel] = float64(variance) * float64(value.Count-1)
			}
		}
	}

	return result, nil
}
