//go:build !js

package lightmapper

import (
	"context"
	"encoding/binary"
	"errors"
	"math"
	"reflect"
	"testing"
)

type bakeBatchCall struct {
	IDs     []uint64
	Options Options
}
type bakeBatchExecutor struct {
	calls          []bakeBatchCall
	badCount, fail int
}

func (*bakeBatchExecutor) Device() GPUDevice {
	return GPUDevice{
		Adapter:                         "batch-test",
		MaxBufferSize:                   128 << 20,
		MaxStorageBufferBindingSize:     128 << 20,
		MaxStorageBuffersPerShaderStage: 4,
	}
}
func (*bakeBatchExecutor) SceneUploads() uint64           { return 0 }
func (*bakeBatchExecutor) TraceRayCounts() GPUTraceCounts { return GPUTraceCounts{} }
func (*bakeBatchExecutor) Close() error                   { return nil }

// This deterministic wire executor depends on identity and per-job options,
// independently of the batch planner's private output ranges.
func (executor *bakeBatchExecutor) Run(_ context.Context, _ GPUScene, tile GPUTile) ([]byte, error) {
	call := bakeBatchCall{Options: tile.Options}

	data := make([]byte, int(tile.Count)*gpuResultWords*gpuWordBytes)
	for index := range int(tile.Count) {
		offset := (gpuTileHeaderWords + index*gpuReceiverWords) * gpuWordBytes
		identity := uint64(
			binary.LittleEndian.Uint32(tile.Data[offset:]),
		) | uint64(
			binary.LittleEndian.Uint32(tile.Data[offset+gpuWordBytes:]),
		)<<32
		call.IDs = append(call.IDs, identity)
		value := float32((identity^uint64(tile.Options.Seed))%73+1) / 32

		for channel := range 3 {
			for kernel := range 4 {
				mean := value * float32(kernel+1)
				binary.LittleEndian.PutUint32(data[(index*gpuResultWords+kernel*3+channel)*gpuWordBytes:], math.Float32bits(mean))
				binary.LittleEndian.PutUint32(data[(index*gpuResultWords+12+kernel*3+channel)*gpuWordBytes:], math.Float32bits(mean*mean))
			}
		}

		binary.LittleEndian.PutUint32(data[(index*gpuResultWords+33)*gpuWordBytes:], tile.Options.Samples)
		binary.LittleEndian.PutUint32(data[(index*gpuResultWords+55)*gpuWordBytes:], 3)
	}

	executor.calls = append(executor.calls, call)
	if executor.fail > 0 && len(executor.calls) == executor.fail {
		return nil, ErrUnsupported
	}

	if executor.badCount > 0 && len(executor.calls) == executor.badCount {
		binary.LittleEndian.PutUint32(data[33*gpuWordBytes:], tile.Options.Samples-1)
	}

	return data, nil
}

func bakeBatchesFixture(t *testing.T, executor *bakeBatchExecutor) (*GPUWorker, *PreparedScene, []BakeBatch) {
	t.Helper()
	scene := gpuFixtureScene(t, Scene{Environment: Environment{Sky: Vec3{.2, .15, .1}}})
	worker := &GPUWorker{ctx: t.Context(), device: executor.Device(), executor: executor, options: GPUWorkerOptions{MemoryBytes: 128 << 20}}
	t.Cleanup(func() { _ = worker.Close() })

	batches := []BakeBatch{
		{
			Receivers: []Receiver{gpuFixtureReceiver(7, 0), gpuFixtureReceiver(19, 1), gpuFixtureReceiver(100, 2)},
			Options:   Options{Samples: 32, Bounces: 3, Seed: 29, RouletteDepth: 1},
		},
		{
			Receivers: []Receiver{gpuFixtureReceiver(7, 3), gpuFixtureReceiver(53, 4)},
			Options:   Options{Samples: 16, Bounces: 2, Seed: 71, Outputs: OutputRGB},
		},
	}

	return worker, scene, batches
}

func TestBakeBatchesMatchIndependentResultsAndMemorySplits(t *testing.T) {
	t.Parallel()

	executor := &bakeBatchExecutor{}
	worker, scene, batches := bakeBatchesFixture(t, executor)

	results, err := worker.BakeBatches(t.Context(), scene, batches)
	if err != nil {
		t.Fatal(err)
	}

	for index, batch := range batches {
		separate, err := worker.Bake(t.Context(), scene, batch.Receivers, batch.Options)
		if err != nil || !reflect.DeepEqual(results[index], separate) {
			t.Fatal("batch changed independent identity/options/statistics", index, err)
		}
	}

	packed, err := worker.scenePacket(scene)
	if err != nil {
		t.Fatal(err)
	}

	tile, err := PackGPUTile(scene, batches[0].Receivers[:1], batches[0].Options)
	if err != nil {
		t.Fatal(err)
	}

	worker.options.MemoryBytes = uint64(
		76 + len(
			packed.Data,
		) + len(
			tile.Data,
		) + gpuPathHeaderBytes + (gpuPathWords+gpuPathQueueCount)*gpuWordBytes + 2*gpuResultWords*gpuWordBytes,
	)

	split, err := worker.BakeBatches(t.Context(), scene, batches)
	if err != nil || !reflect.DeepEqual(split, results) {
		t.Fatal("memory-driven splitting changed independent results", err)
	}

	for _, call := range executor.calls[len(executor.calls)-5:] {
		if len(call.IDs) != 1 {
			t.Fatal("fixture did not force single-owner device splits")
		}
	}
}

func TestBakeBatchesCompleteAdmissionPrecedesFirstExecutorCall(t *testing.T) {
	t.Parallel()

	for _, mutate := range []func([]BakeBatch){
		func(batches []BakeBatch) { batches[1].Receivers = nil },
		func(batches []BakeBatch) { batches[1].Receivers[1].ID = batches[1].Receivers[0].ID },
		func(batches []BakeBatch) { batches[1].Receivers[0].Coverage = 0 },
		func(batches []BakeBatch) { batches[1].Receivers[0].Patches[0].Positions[0][0] = float32(math.NaN()) },
		func(batches []BakeBatch) { batches[1].Options.Bounces = 33 },
	} {
		executor := &bakeBatchExecutor{}
		worker, scene, batches := bakeBatchesFixture(t, executor)
		mutate(batches)

		if results, err := worker.BakeBatches(t.Context(), scene, batches); err == nil || results != nil || len(executor.calls) != 0 {
			t.Fatal("invalid later page reached GPU or exposed partial output", err, len(executor.calls))
		}
	}
}

func TestBakeBatchesRejectBadCountsFailuresAndCancellationWithoutPartialResults(t *testing.T) {
	t.Parallel()

	for _, executor := range []*bakeBatchExecutor{{badCount: 1}, {fail: 2}} {
		worker, scene, batches := bakeBatchesFixture(t, executor)
		if results, err := worker.BakeBatches(t.Context(), scene, batches); err == nil || results != nil {
			t.Fatal("invalid output or execution failure exposed partial batch", err)
		}
	}

	executor := &bakeBatchExecutor{}
	worker, scene, batches := bakeBatchesFixture(t, executor)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if results, err := worker.BakeBatches(
		ctx,
		scene,
		batches,
	); !errors.Is(err, context.Canceled) || results != nil ||
		len(executor.calls) != 0 {
		t.Fatal("canceled roster reached executor or published results", err)
	}
}
