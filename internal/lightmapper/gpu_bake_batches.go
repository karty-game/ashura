//go:build !js

package lightmapper

import (
	"context"
	"runtime"
	"sync"
)

// BakeBatch owns one independently identified fixed-count receiver domain.
// Inputs must remain immutable until BakeBatches returns. Options, seeds and
// receiver identities retain the same meaning as a separate Bake call.
type BakeBatch struct {
	Receivers []Receiver
	Options   Options
}

type preparedBakeBatch struct {
	result Result
	tiles  []preparedBakeTile
}

type preparedBakeTile struct {
	tile      GPUTile
	receivers []Receiver
	target    []Estimate
}

// BakeBatches admits and prepares every independent batch before submitting any
// GPU work. Preparation uses the available Go workers; serial GPU dispatch
// overlaps one previous result decoder. Every worker joins before return, and
// errors return no partial results. Memory tiling preserves complete counts,
// receiver order, seeds and per-batch result provenance.
func (worker *GPUWorker) BakeBatches(ctx context.Context, scene *PreparedScene, batches []BakeBatch) ([]Result, error) {
	if worker == nil || ctx == nil || scene == nil || len(batches) == 0 {
		return nil, ErrInvalid
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	packed, err := worker.scenePacket(scene)
	if err != nil {
		return nil, err
	}

	device := worker.Device()
	if uint64(len(packed.Data)) >= worker.options.MemoryBytes || uint64(len(packed.Data)) > device.MaxStorageBufferBindingSize {
		return nil, ErrBounds
	}

	prepared, err := worker.prepareBakeBatches(ctx, scene, packed, batches)
	if err != nil {
		return nil, err
	}

	results := make([]Result, len(prepared))

	var tiles []preparedBakeTile

	for index := range prepared {
		results[index] = prepared[index].result
		tiles = append(tiles, prepared[index].tiles...)
		prepared[index].tiles = nil
	}

	if err = worker.runPreparedBakeTiles(ctx, packed, tiles); err != nil {
		return nil, err
	}

	return results, nil
}

func (worker *GPUWorker) prepareBakeBatches(
	ctx context.Context, scene *PreparedScene, packed GPUScene, batches []BakeBatch,
) ([]preparedBakeBatch, error) {
	prepared := make([]preparedBakeBatch, len(batches))
	errors := make([]error, len(batches))

	count := min(len(batches), runtime.GOMAXPROCS(0))
	if count == 1 {
		for index, batch := range batches {
			prepared[index], errors[index] = worker.prepareBakeBatch(ctx, scene, packed, batch)
		}
	} else {
		jobs := make(chan int)

		var workers sync.WaitGroup

		for range count {
			workers.Go(func() {
				for index := range jobs {
					prepared[index], errors[index] = worker.prepareBakeBatch(ctx, scene, packed, batches[index])
				}
			})
		}

		for index := range batches {
			jobs <- index
		}

		close(jobs)
		workers.Wait()
	}

	for _, err := range errors {
		if err != nil {
			return nil, err
		}
	}

	return prepared, nil
}
