//go:build !js

package lightmapper

import (
	"context"
	"errors"
)

func (worker *GPUWorker) runPreparedBakeTiles(ctx context.Context, scene GPUScene, tiles []preparedBakeTile) error {
	var (
		pending    <-chan error
		failure    error
		dispatched bool
	)

	for index := range tiles {
		if err := ctx.Err(); err != nil {
			failure = err

			break
		}

		tile := tiles[index]

		data, err := worker.Run(ctx, scene, tile.tile)
		if err == nil {
			dispatched = true
		}

		tiles[index].tile.Data = nil

		if pending != nil {
			failure = <-pending
			pending = nil

			if failure != nil {
				break
			}
		}

		if err != nil {
			failure = err

			break
		}

		pending = decodePreparedBakeAsync(ctx, data, tile.receivers, tile.target, tile.tile.Options.Samples)
	}

	if pending != nil {
		if err := <-pending; failure == nil {
			failure = err
		}
	}

	if failure != nil && dispatched && !errors.Is(failure, errGPUWorkerBusy) {
		worker.fail(failure)
		_ = worker.Close()
	}

	return failure
}

func decodePreparedBakeAsync(ctx context.Context, data []byte, receivers []Receiver, target []Estimate, samples uint32) <-chan error {
	done := make(chan error, 1)

	go func() {
		if err := ctx.Err(); err != nil {
			done <- err

			return
		}

		err := decodeGPUEstimatesInto(data, receivers, target)
		if err == nil && !gpuEstimateCountsMatch(target, samples) {
			err = ErrInvalid
		}

		if err == nil {
			err = ctx.Err()
		}

		done <- err
	}()

	return done
}
