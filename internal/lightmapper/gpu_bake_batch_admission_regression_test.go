//go:build !js

package lightmapper

import (
	"context"
	"errors"
	"testing"
)

func TestBakeBatchesRejectDuplicateIDsBeforeEarlyPacketSizeSplit(t *testing.T) {
	t.Parallel()

	executor := &bakeBatchExecutor{}
	worker, scene, batches := bakeBatchesFixture(t, executor)

	const records = 3

	const patchesPerReceiver = gpuMaximumBytes/(records*gpuPatchWords*gpuWordBytes) + 1

	const patchArea = 1.0 / (1 << 20)

	patch := gpuFixtureReceiver(7, 0).Patches[0]
	patch.Area = patchArea

	patches := make([]ReceiverPatch, patchesPerReceiver)
	for index := range patches {
		patches[index] = patch
	}

	receivers := make([]Receiver, records)
	for index, identity := range []uint64{7, 19, 7} {
		receivers[index] = gpuFixtureReceiver(identity, float32(index))
		receivers[index].Patches = patches
		receivers[index].Coverage = float32(patchesPerReceiver) * patchArea
	}

	// The total packet is rejected before PackGPUTile reaches its local ID map,
	// while every physical half fits that packet bound. Logical IDs must be
	// admitted before this split can separate the two copies of identity 7.
	if _, err := gpuTileWordCount(receivers); !errors.Is(err, ErrBounds) {
		t.Fatal("fixture did not reach early packet-size rejection", err)
	}

	for _, half := range [][]Receiver{receivers[:1], receivers[1:]} {
		if _, err := gpuTileWordCount(half); err != nil {
			t.Fatal("fixture physical half did not fit packet bounds", err)
		}
	}

	batches[0].Receivers = receivers

	result, err := worker.BakeBatches(t.Context(), scene, batches[:1])
	if !errors.Is(err, ErrInvalid) || result != nil || len(executor.calls) != 0 {
		t.Fatal("duplicate logical ID escaped early physical splitting", err, len(executor.calls))
	}
}

type cancelAfterBakeDispatch struct {
	*bakeBatchExecutor

	cancel context.CancelFunc
}

func (executor cancelAfterBakeDispatch) Run(ctx context.Context, scene GPUScene, tile GPUTile) ([]byte, error) {
	data, err := executor.bakeBatchExecutor.Run(ctx, scene, tile)
	executor.cancel()

	return data, err
}

func TestBakeBatchDecodeAndPostDispatchCancellationPoisonGeneration(t *testing.T) {
	t.Parallel()

	for _, mode := range []string{"invalid-count", "cancel-after-dispatch"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()

			executor := &bakeBatchExecutor{}
			worker, scene, batches := bakeBatchesFixture(t, executor)

			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()

			expected := ErrInvalid

			if mode == "invalid-count" {
				executor.badCount = 1
			} else {
				expected = context.Canceled
				worker.executor = cancelAfterBakeDispatch{bakeBatchExecutor: executor, cancel: cancel}
			}

			if result, err := worker.BakeBatches(ctx, scene, batches[:1]); !errors.Is(err, expected) || result != nil {
				t.Fatal("invalid/canceled result was admitted", result, err)
			}

			calls := len(executor.calls)
			if calls == 0 {
				t.Fatal("fixture did not dispatch before failing")
			}

			_, err := worker.Bake(t.Context(), scene, batches[0].Receivers, batches[0].Options)
			if !errors.Is(err, expected) || len(executor.calls) != calls {
				t.Fatal("failed generation accepted another dispatched bake", err, calls, len(executor.calls))
			}
		})
	}
}
