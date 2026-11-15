//go:build !js

package lightmapper

import (
	"context"
	"encoding/binary"
	"errors"
)

// packSeededGPUVerification adds an explicit seed for each unchanged receiver.
// The private tail follows every padded directional normal; zero offset retains
// the legacy single-job seed. Identity and random-dimension addressing stay intact.
func packSeededGPUVerification(
	scene *PreparedScene,
	receivers []Receiver,
	options Options,
	normals [][]Vec3,
	seeds []uint32,
) (GPUTile, error) {
	if len(seeds) != 0 && len(seeds) != len(receivers) {
		return GPUTile{}, ErrInvalid
	}

	tile, err := PackGPUVerification(scene, receivers, options, normals)
	if err != nil || len(seeds) == 0 {
		return tile, err
	}

	if uint64(len(tile.Data))+uint64(len(seeds))*gpuWordBytes > gpuMaximumBytes {
		return GPUTile{}, ErrBounds
	}

	//nolint:gosec // G115: the complete 128 MiB packet bound admits both word offsets and counts before narrowing.
	offset, count := uint32(len(tile.Data)/gpuWordBytes), uint32(len(seeds))
	binary.LittleEndian.PutUint32(tile.Data[6*gpuWordBytes:], offset)
	binary.LittleEndian.PutUint32(tile.Data[7*gpuWordBytes:], count)

	for _, seed := range seeds {
		tile.Data = binary.LittleEndian.AppendUint32(tile.Data, seed)
	}

	return tile, nil
}

type verificationGPURequest struct {
	receivers []Receiver
	normals   [][]Vec3
	seeds     []uint32
	options   Options
}

type verificationGPUInterval struct{ first, last int }

// verificationIntervals admits every split before the first executor call. Only
// bounded interval descriptors survive this pass, so complete-batch admission
// does not retain all packed staging buffers at once.
func (worker *GPUWorker) verificationIntervals(
	ctx context.Context,
	scene *PreparedScene,
	packed GPUScene,
	request verificationGPURequest,
) ([]verificationGPUInterval, error) {
	var intervals []verificationGPUInterval

	var admit func(int, int) error

	admit = func(first, last int) error {
		if err := ctx.Err(); err != nil {
			return err
		}

		var seeds []uint32

		if len(request.seeds) > 0 {
			seeds = request.seeds[first:last]
		}

		tile, err := packSeededGPUVerification(scene, request.receivers[first:last], request.options, request.normals[first:last], seeds)
		if err == nil {
			err = worker.admit(packed, tile)
		}

		if errors.Is(err, ErrBounds) && last-first > 1 {
			middle := first + (last-first)/gpuBatchSplit

			if err = admit(first, middle); err != nil {
				return err
			}

			return admit(middle, last)
		}

		if err != nil {
			return err
		}

		intervals = append(intervals, verificationGPUInterval{first, last})

		return nil
	}

	if err := admit(0, len(request.receivers)); err != nil {
		return nil, err
	}

	return intervals, nil
}

func (worker *GPUWorker) runVerificationIntervals(
	ctx context.Context,
	scene *PreparedScene,
	packed GPUScene,
	request verificationGPURequest,
	intervals []verificationGPUInterval,
) ([]ResponseEstimate, error) {
	result := make([]ResponseEstimate, 0, len(request.receivers))

	for _, interval := range intervals {
		batch, probes := request.receivers[interval.first:interval.last], request.normals[interval.first:interval.last]

		var seeds []uint32

		if len(request.seeds) > 0 {
			seeds = request.seeds[interval.first:interval.last]
		}

		tile, err := packSeededGPUVerification(scene, batch, request.options, probes, seeds)
		if err != nil {
			return nil, err
		}

		data, err := worker.Run(ctx, packed, tile)
		if err != nil {
			return nil, err
		}

		estimates, err := DecodeGPUResponses(data, batch, probes, tile.ProbeCount)
		if err != nil {
			return nil, err
		}

		if !gpuResponseCountMatches(estimates, request.options.Samples) {
			return nil, ErrInvalid
		}

		result = append(result, estimates...)
	}

	return result, nil
}

func (worker *GPUWorker) bakeSeededResponses(
	ctx context.Context,
	scene *PreparedScene,
	request verificationGPURequest,
) ([]ResponseEstimate, error) {
	if worker == nil || ctx == nil || scene == nil || len(request.receivers) == 0 || len(request.receivers) != len(request.normals) ||
		len(request.seeds) > 0 && len(request.seeds) != len(request.receivers) {
		return nil, ErrInvalid
	}

	var err error

	request.options, err = normalizeOptions(request.options)
	if err != nil {
		return nil, err
	}

	packed, err := worker.scenePacket(scene)
	if err != nil {
		return nil, err
	}

	intervals, err := worker.verificationIntervals(ctx, scene, packed, request)
	if err != nil {
		return nil, err
	}

	return worker.runVerificationIntervals(ctx, scene, packed, request, intervals)
}
