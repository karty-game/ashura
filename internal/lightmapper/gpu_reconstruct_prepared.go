//go:build !js

package lightmapper

import (
	"context"
	"math"
)

// ReconstructPrepared consumes sealed static support and admits the dynamic
// estimates before dispatch. Its support, edge order and output match Reconstruct.
func (worker *GPUWorker) ReconstructPrepared(
	ctx context.Context, scene *PreparedScene, receivers []Receiver, estimates []Estimate,
	support *ReconstructionSupport, options ReconstructionOptions,
) ([]Estimate, error) {
	if worker == nil || ctx == nil || scene == nil || support == nil || len(receivers) != len(support.identities) {
		return nil, ErrInvalid
	}

	if !reconstructionValidOptions(options) {
		return nil, errReconstruction
	}

	selectedReceivers, selectedHalo, err := support.prepareSamples(scene.Source().Space, receivers, estimates)
	if err != nil {
		return nil, err
	}

	if err = ctx.Err(); err != nil {
		return nil, err
	}

	var reconstructed []Estimate

	if len(selectedReceivers) > 0 {
		graph := &reconstructionGraph{
			halo: selectedHalo, offsets: support.offsets, starts: support.starts, unitLinks: support.targets,
			marks: make([]uint32, len(selectedHalo)), local: make([]uint32, len(selectedHalo)),
		}

		reconstructed, err = worker.reconstructGraphTiles(ctx, scene, selectedReceivers, graph, options)
		if err != nil {
			return nil, err
		}
	}

	return mergeReconstructionFrames(estimates, reconstructed, support.original)
}

func (support *ReconstructionSupport) prepareSamples(
	space string, receivers []Receiver, estimates []Estimate,
) ([]Receiver, []ReconstructionSample, error) {
	if len(receivers) != len(estimates) || len(receivers) != len(support.identities) {
		return nil, nil, ErrInvalid
	}

	selectedReceivers := make([]Receiver, 0, len(support.original))
	selectedHalo := make([]ReconstructionSample, 0, len(support.original))

	for index, receiver := range receivers {
		sample, err := reconstructionSample(space, receiver, estimates[index])
		if err != nil {
			return nil, nil, err
		}

		omitted := support.retained[index] == omittedReconstructionFrame
		if receiver.ID != support.identities[index] || reconstructionReceiverGuide(receiver) != support.guides[index] ||
			omitted != (sample.Tangent == (Vec3{})) || !sample.Finalized || sample.Samples < 2 ||
			sample.Samples > math.MaxUint32 || !reconstructionValidValues(sample) || !positiveVec(sample.Direct) ||
			!positiveVec(sample.Environment) || !positiveVec(sample.Reflected) || !omitted && !reconstructionValidSample(sample) {
			return nil, nil, errReconstruction
		}

		if !omitted {
			selectedReceivers = append(selectedReceivers, receiver)
			selectedHalo = append(selectedHalo, sample)
		}
	}

	return selectedReceivers, selectedHalo, nil
}
