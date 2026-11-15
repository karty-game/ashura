//go:build !js

package lightmapper

import (
	"context"
	"errors"
)

func (worker *GPUWorker) prepareBakeBatch(
	ctx context.Context, scene *PreparedScene, packed GPUScene, batch BakeBatch,
) (preparedBakeBatch, error) {
	var prepared preparedBakeBatch

	if err := ctx.Err(); err != nil {
		return prepared, err
	}

	if err := admitBakeBatchReceivers(batch.Receivers); err != nil {
		return prepared, err
	}

	effective, err := normalizeOptions(batch.Options)
	if err != nil {
		return prepared, err
	}

	identity, err := ReceiverHash(batch.Receivers)
	if err != nil {
		return prepared, err
	}

	prepared.result = Result{
		Schema: ResultSchema, Algorithm: Algorithm, SceneHash: scene.Hash(), ReceiverHash: identity,
		Options: effective, Backend: worker.Device().Adapter, Estimates: make([]Estimate, len(batch.Receivers)),
	}

	prepared.tiles, err = worker.prepareBakeTiles(ctx, scene, packed, batch.Receivers, effective, prepared.result.Estimates)
	if err != nil {
		return preparedBakeBatch{}, err
	}

	return prepared, nil
}

// Logical identity admission precedes size-driven physical packet splitting.
// A duplicate separated by a device split must never enter two valid packets.
func admitBakeBatchReceivers(receivers []Receiver) error {
	if len(receivers) == 0 || len(receivers) > 1<<20 {
		return ErrInvalid
	}

	identities := make(map[uint64]struct{}, len(receivers))
	for _, receiver := range receivers {
		if _, duplicate := identities[receiver.ID]; duplicate {
			return ErrInvalid
		}

		identities[receiver.ID] = struct{}{}
	}

	return nil
}

func (worker *GPUWorker) prepareBakeTiles(
	ctx context.Context, scene *PreparedScene, packed GPUScene, receivers []Receiver, options Options, target []Estimate,
) ([]preparedBakeTile, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	tile, err := PackGPUTile(scene, receivers, options)
	if err == nil {
		err = worker.admit(packed, tile)
	}

	if errors.Is(err, ErrBounds) && len(receivers) > 1 {
		middle := len(receivers) / gpuBatchSplit

		first, firstErr := worker.prepareBakeTiles(ctx, scene, packed, receivers[:middle], options, target[:middle])
		if firstErr != nil {
			return nil, firstErr
		}

		last, lastErr := worker.prepareBakeTiles(ctx, scene, packed, receivers[middle:], options, target[middle:])
		if lastErr != nil {
			return nil, lastErr
		}

		return append(first, last...), nil
	}

	if err != nil {
		return nil, err
	}

	return []preparedBakeTile{{tile: tile, receivers: receivers, target: target}}, nil
}
