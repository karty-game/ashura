package lightmapper

import (
	"errors"
	"math"
)

// ReconstructionSample is an immutable finalized estimate from a tile artifact.
// All neighbors needed by an interior sample must be in the bounded halo input.
type ReconstructionSample struct {
	ID                             uint64
	Space                          string
	Position, Normal, Tangent      Vec3
	Handedness                     float32
	Side                           int32
	Mean, Variance                 Moments
	Samples                        uint64
	Finalized                      bool
	Direct, Environment, Reflected Vec3
}

// ReconstructionEdge is explicit surface adjacency. Visible must be established
// by preparation; spatial or UV proximity alone must never create this edge.
type ReconstructionEdge struct {
	From, To uint64
	Weight   float32
	Visible  bool
}

// ReconstructionOptions defines one immutable filtering pass. Radius and plane
// tolerance use world units; angular thresholds compare normalized frames.
type ReconstructionOptions struct {
	Radius, PlaneTolerance, NormalCos, TangentCos, NoiseScale, Strength float32
	// MaximumHalo is an optional scheduling ceiling. Zero uses the actual packet,
	// storage-binding and worker memory bounds without an additional receiver cap.
	MaximumHalo uint32
}

var errReconstruction = errors.New("lightmapper: invalid or unfinished reconstruction halo")

// ReconstructLinear is the deterministic CPU reference for one filtering pass.
// It returns only interior IDs, in requested order; it never mutates halo input.
// Cross-tile edges obey the same contract as intra-tile edges. A later pass must
// read completely finalized previous-pass artifacts, never partial current output.
func ReconstructLinear(
	interior []uint64,
	halo []ReconstructionSample,
	edges []ReconstructionEdge,
	options ReconstructionOptions,
) ([]Moments, error) {
	if !reconstructionValidOptions(options) || !reconstructionHaloFits(uint64(len(halo)), options) {
		return nil, errReconstruction
	}

	index := make(map[uint64]int, len(halo))
	for i, sample := range halo {
		if _, exists := index[sample.ID]; exists || !sample.Finalized || sample.Samples == 0 || !reconstructionValidSample(sample) {
			return nil, errReconstruction
		}

		index[sample.ID] = i
	}

	neighbors := make(map[uint64][]ReconstructionEdge)

	for _, edge := range edges {
		if !receiverFinite(edge.Weight) || edge.Weight <= 0 {
			return nil, errReconstruction
		}

		if _, exists := index[edge.From]; !exists {
			return nil, errReconstruction
		}

		if _, exists := index[edge.To]; !exists {
			return nil, errReconstruction
		}

		neighbors[edge.From] = append(neighbors[edge.From], edge)
	}

	result := make([]Moments, len(interior))
	for interiorIndex, receiverID := range interior {
		centerIndex, exists := index[receiverID]
		if !exists {
			return nil, errReconstruction
		}

		result[interiorIndex] = reconstructionMean(halo[centerIndex], halo, index, neighbors[receiverID], options)
	}

	return result, nil
}

func reconstructionWeight(center, neighbor ReconstructionSample, options ReconstructionOptions) float64 {
	normal, tangent, distance, eligible := reconstructionGuideFactors(
		reconstructionSampleGuide(center),
		reconstructionSampleGuide(neighbor),
		options,
	)
	if center.Space != neighbor.Space || !eligible {
		return 0
	}

	var contrast float64

	for channel := range 3 {
		difference := float64(center.Mean[0][channel] - neighbor.Mean[0][channel])

		noise := float64(
			center.Variance[0][channel],
		)/float64(
			center.Samples,
		) + float64(
			neighbor.Variance[0][channel],
		)/float64(
			neighbor.Samples,
		)
		if noise == 0 {
			if difference != 0 {
				return 0
			}

			continue
		}

		contrast += difference * difference / (float64(options.NoiseScale) * noise)
	}

	return math.Exp(
		-.5*contrast,
	) * float64(
		max(float32(0), normal)*max(float32(0), tangent),
	) / (1 + float64(distance/(options.Radius*options.Radius)))
}

func reconstructionValidOptions(options ReconstructionOptions) bool {
	for _, value := range []float32{options.Radius, options.PlaneTolerance, options.NormalCos, options.TangentCos, options.NoiseScale, options.Strength} {
		if !receiverFinite(value) {
			return false
		}
	}

	return options.Radius > 0 && options.PlaneTolerance >= 0 && options.NormalCos >= 0 &&
		options.NormalCos <= 1 &&
		options.TangentCos >= 0 &&
		options.TangentCos <= 1 &&
		options.NoiseScale > 0 &&
		options.Strength >= 0 &&
		options.Strength <= 1
}

func reconstructionHaloFits(count uint64, options ReconstructionOptions) bool {
	return count <= math.MaxUint32 && (options.MaximumHalo == 0 || count <= uint64(options.MaximumHalo))
}

func reconstructionValidSample(sample ReconstructionSample) bool {
	return reconstructionValidValues(sample) && math.Abs(float64(Dot(sample.Tangent, sample.Tangent)-1)) <= 1e-3
}

// reconstructionValidValues checks the immutable numeric estimate independently
// of its averaged tangent. Opposing valid patch frames can have a zero average;
// such a receiver cannot contribute to a tangent-weighted reconstruction.
func reconstructionValidValues(sample ReconstructionSample) bool {
	for _, vector := range []Vec3{sample.Position, sample.Normal, sample.Tangent} {
		for _, value := range vector {
			if !receiverFinite(value) {
				return false
			}
		}
	}

	if !receiverFinite(sample.Handedness) || math.Abs(float64(Dot(sample.Normal, sample.Normal)-1)) > 1e-3 {
		return false
	}

	for kernel := range 4 {
		for channel := range 3 {
			if !receiverFinite(sample.Mean[kernel][channel]) || sample.Mean[kernel][channel] < 0 ||
				!receiverFinite(sample.Variance[kernel][channel]) ||
				sample.Variance[kernel][channel] < 0 {
				return false
			}
		}
	}

	return true
}

func reconstructionMean(
	center ReconstructionSample,
	halo []ReconstructionSample,
	index map[uint64]int,
	edges []ReconstructionEdge,
	options ReconstructionOptions,
) Moments {
	var result Moments

	var sums [4][3]float64

	for kernel := range 4 {
		for channel := range 3 {
			sums[kernel][channel] = float64(center.Mean[kernel][channel])
		}
	}

	total := float64(1)

	for _, edge := range edges {
		if !edge.Visible || edge.To == center.ID {
			continue
		}

		neighbor := halo[index[edge.To]]
		weight := float64(edge.Weight) * reconstructionWeight(center, neighbor, options)
		total += weight

		for kernel := range 4 {
			for channel := range 3 {
				sums[kernel][channel] += float64(neighbor.Mean[kernel][channel]) * weight
			}
		}
	}

	for kernel := range 4 {
		for channel := range 3 {
			old := float64(center.Mean[kernel][channel])
			result[kernel][channel] = float32(old + float64(options.Strength)*(sums[kernel][channel]/total-old))
		}
	}

	return result
}

type reconstructionGuide struct {
	Position, Normal, Tangent Vec3
	Handedness                float32
	Side                      int32
}

func reconstructionSampleGuide(sample ReconstructionSample) reconstructionGuide {
	return reconstructionGuide{
		Position:   sample.Position,
		Normal:     sample.Normal,
		Tangent:    sample.Tangent,
		Handedness: sample.Handedness,
		Side:       sample.Side,
	}
}
func reconstructionReceiverGuide(receiver Receiver) reconstructionGuide {
	return reconstructionGuide{
		Position:   receiver.Position,
		Normal:     receiver.Normal,
		Tangent:    receiver.Tangent,
		Handedness: receiver.Handedness,
		Side:       receiver.Side,
	}
}

//nolint:dogsled // The shared factor calculation also supplies the exact geometry-only eligibility predicate.
func reconstructionGuidesEligible(center, neighbor reconstructionGuide, options ReconstructionOptions) bool {
	_, _, _, eligible := reconstructionGuideFactors(center, neighbor, options)

	return eligible
}
func reconstructionGuideFactors(center, neighbor reconstructionGuide, options ReconstructionOptions) (float32, float32, float32, bool) {
	if center.Side != neighbor.Side || center.Handedness != neighbor.Handedness {
		return 0, 0, 0, false
	}

	normal, tangent := Dot(center.Normal, neighbor.Normal), Dot(center.Tangent, neighbor.Tangent)
	if normal < options.NormalCos || tangent < options.TangentCos {
		return 0, 0, 0, false
	}

	delta := Sub(neighbor.Position, center.Position)

	distance := Dot(delta, delta)
	if distance > options.Radius*options.Radius || float32(math.Abs(float64(Dot(delta, center.Normal)))) > options.PlaneTolerance {
		return 0, 0, 0, false
	}

	return normal, tangent, distance, true
}
