//go:build !js

package lightmapper

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"math"
	"os"
	"testing"
	"time"
)

type verificationSeedExecutor struct{ calls int }

func (*verificationSeedExecutor) Device() GPUDevice {
	return GPUDevice{MaxBufferSize: 128 << 20, MaxStorageBufferBindingSize: 128 << 20, MaxStorageBuffersPerShaderStage: 4}
}
func (*verificationSeedExecutor) SceneUploads() uint64           { return 0 }
func (*verificationSeedExecutor) TraceRayCounts() GPUTraceCounts { return GPUTraceCounts{} }
func (*verificationSeedExecutor) Close() error                   { return nil }
func (executor *verificationSeedExecutor) Run(_ context.Context, _ GPUScene, tile GPUTile) ([]byte, error) {
	executor.calls++
	stride := int(tile.ProbeCount)*9 + 2
	data := make([]byte, int(tile.Count)*stride*gpuWordBytes)
	word := func(offset int) uint32 { return binary.LittleEndian.Uint32(tile.Data[offset*gpuWordBytes:]) }

	for receiver := range int(tile.Count) {
		seed := tile.Options.Seed
		if word(6) != 0 {
			seed = word(int(word(6)) + receiver)
		}

		identity := word(gpuTileHeaderWords + receiver*gpuReceiverWords)
		for probe := range int(tile.ProbeCount) {
			value := float32((seed^identity)%103+uint32(probe)+1) / 128
			for channel := range 3 {
				binary.LittleEndian.PutUint32(data[(receiver*stride+probe*3+channel)*4:], math.Float32bits(value))
				binary.LittleEndian.PutUint32(
					data[(receiver*stride+int(tile.ProbeCount)*3+probe*3+channel)*4:],
					math.Float32bits(value*value),
				)
			}
		}

		binary.LittleEndian.PutUint32(data[(receiver*stride+int(tile.ProbeCount)*9)*4:], tile.Options.Samples)
		binary.LittleEndian.PutUint32(data[(receiver*stride+int(tile.ProbeCount)*9+1)*4:], 3)
	}

	return data, nil
}

func verificationBatchesFixture(t *testing.T) (*PreparedScene, []VerificationBatch) {
	t.Helper()
	scene := gpuFixtureScene(
		t,
		Scene{
			Materials:   []Material{{Reflectance: Vec3{.4, .5, .6}}, {Reflectance: Vec3{.6, .5, .4}}},
			Environment: Environment{Sky: Vec3{.6, .4, .2}, Ground: Vec3{.03, .04, .05}},
		},
	)

	batches := make([]VerificationBatch, 2)
	for page := range batches {
		batch := &batches[page]
		batch.Options = Options{Outputs: OutputRGB | OutputRNM, Samples: 32, Bounces: 2, Seed: 71}

		batch.Policy = VerificationPolicy{
			Samples:               16,
			MaximumReceivers:      2,
			AbsoluteTolerance:     10,
			RelativeTolerance:     0,
			Confidence:            2,
			RequireMaterialDomain: true,
			MaterialDomains: []MaterialNormalDomain{
				{Material: uint32(page), Identity: "prepared-domain", Normals: []Vec3{{.6, 0, .8}}},
			},
		}
		for index := range 3 {
			receiver := gpuFixtureReceiver(uint64(page*10+index+1), float32(index))
			receiver.Patches[0].Material = uint32(page)
			batch.Receivers = append(batch.Receivers, receiver)
			batch.Estimates = append(batch.Estimates, Estimate{ReceiverID: receiver.ID, Count: 32})
		}
	}

	return scene, batches
}

func verificationBatchJSON(t *testing.T, value any) []byte {
	t.Helper()

	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}

	return data
}

func compareVerificationBatches(t *testing.T, worker *GPUWorker, scene *PreparedScene, batches []VerificationBatch) {
	t.Helper()

	separate := make([]VerificationReport, len(batches))
	for index, batch := range batches {
		var err error

		separate[index], err = worker.VerifyRNM(t.Context(), scene, batch.Receivers, batch.Estimates, batch.Options, batch.Policy)
		if err != nil {
			t.Fatal(err)
		}
	}

	if separate[0].Seed == separate[1].Seed {
		t.Fatal("fixture pages did not derive independent seeds")
	}

	combined, err := worker.VerifyRNMBatches(t.Context(), scene, batches)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(verificationBatchJSON(t, separate), verificationBatchJSON(t, combined)) {
		t.Fatal("batched verification changed a report, selected identity, seed, coverage, normal or response float")
	}
	// Force memory subdivision through a ceiling that admits one selected receiver.
	plan, err := prepareVerification(
		scene,
		batches[0].Receivers,
		batches[0].Estimates,
		batches[0].Options,
		batches[0].Policy,
		worker.Device().Adapter,
	)
	if err != nil {
		t.Fatal(err)
	}

	packed, err := worker.scenePacket(scene)
	if err != nil {
		t.Fatal(err)
	}

	tile, err := packSeededGPUVerification(scene, plan.Receivers[:1], plan.Options, plan.Normals[:1], []uint32{plan.Options.Seed})
	if err != nil {
		t.Fatal(err)
	}

	previous := worker.options.MemoryBytes

	worker.options.MemoryBytes = uint64(
		64+12+len(packed.Data)+len(tile.Data),
	) + gpuPathHeaderBytes + (gpuPathRecordWords(tile)+gpuPathQueueCount)*gpuWordBytes + 2*gpuOutputRecordWords(
		tile,
	)*gpuWordBytes
	defer func() { worker.options.MemoryBytes = previous }()

	split, err := worker.VerifyRNMBatches(t.Context(), scene, batches)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(verificationBatchJSON(t, separate), verificationBatchJSON(t, split)) {
		t.Fatal("memory subdivision changed independently seeded reports")
	}
}

func TestVerificationBatchPreservesPageSeedsAndReports(t *testing.T) {
	t.Parallel()
	scene, batches := verificationBatchesFixture(t)
	executor := &verificationSeedExecutor{}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	worker := &GPUWorker{
		ctx:      ctx,
		cancel:   cancel,
		executor: executor,
		device:   executor.Device(),
		options:  GPUWorkerOptions{MemoryBytes: 128 << 20},
	}
	compareVerificationBatches(t, worker, scene, batches)

	if executor.calls != 7 {
		t.Fatalf("separate2 + combined1 + split4 calls =7, got%d", executor.calls)
	}
}

func TestVerificationBatchAdmitsEveryPageBeforeDispatch(t *testing.T) {
	t.Parallel()
	scene, batches := verificationBatchesFixture(t)
	batches[1].Estimates[0].ReceiverID++
	executor := &verificationSeedExecutor{}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	worker := &GPUWorker{
		ctx:      ctx,
		cancel:   cancel,
		executor: executor,
		device:   executor.Device(),
		options:  GPUWorkerOptions{MemoryBytes: 128 << 20},
	}
	if _, err := worker.VerifyRNMBatches(t.Context(), scene, batches); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}

	if executor.calls != 0 {
		t.Fatal("invalid later page reached GPU")
	}
}

//nolint:paralleltest // One actual native worker measures independently seeded page batching and forced split behavior.
func TestHardwareGPUVerificationBatchesPreserveResponses(t *testing.T) {
	if os.Getenv("KARTY_LIGHTMAPPER_GPU_TEST") != "1" {
		t.Skip("set KARTY_LIGHTMAPPER_GPU_TEST=1 with hardware GPU access")
	}

	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()

	worker, err := NewGPUWorker(ctx, GPUWorkerOptions{MemoryBytes: 128 << 20})
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = worker.Close() })
	t.Log("hardware", worker.Device().Adapter)
	scene, batches := verificationBatchesFixture(t)
	compareVerificationBatches(t, worker, scene, batches)
	verifyRawSeededResponseEquality(t, worker, scene, batches)
}

func verifyRawSeededResponseEquality(t *testing.T, worker *GPUWorker, scene *PreparedScene, batches []VerificationBatch) {
	t.Helper()

	packed, err := worker.scenePacket(scene)
	if err != nil {
		t.Fatal(err)
	}

	var separate []byte

	request := verificationGPURequest{}

	for _, batch := range batches {
		plan, planErr := prepareVerification(scene, batch.Receivers, batch.Estimates, batch.Options, batch.Policy, worker.Device().Adapter)
		if planErr != nil {
			t.Fatal(planErr)
		}

		tile, packErr := PackGPUVerification(scene, plan.Receivers, plan.Options, plan.Normals)
		if packErr != nil {
			t.Fatal(packErr)
		}

		data, runErr := worker.Run(t.Context(), packed, tile)
		if runErr != nil {
			t.Fatal(runErr)
		}

		separate = append(separate, data...)
		request.receivers = append(request.receivers, plan.Receivers...)

		request.normals = append(request.normals, plan.Normals...)
		for range plan.Receivers {
			request.seeds = append(request.seeds, plan.Options.Seed)
		}

		request.options = plan.Options
	}

	tile, err := packSeededGPUVerification(scene, request.receivers, request.options, request.normals, request.seeds)
	if err != nil {
		t.Fatal(err)
	}

	data, err := worker.Run(t.Context(), packed, tile)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(separate, data) {
		t.Fatal("explicit per-receiver seed batching changed raw native response bytes")
	}
}

func TestVerificationGroupingRetainsTransportAndIdentityOwnership(t *testing.T) {
	t.Parallel()

	for _, overlap := range []bool{false, true} {
		scene, batches := verificationBatchesFixture(t)
		if overlap {
			for index := range batches[1].Receivers {
				batches[1].Receivers[index].ID = batches[0].Receivers[index].ID
				batches[1].Estimates[index].ReceiverID = batches[0].Receivers[index].ID
			}
		} else {
			batches[1].Options.Bounces++
		}

		var groups []verificationBatchGroup

		for index, batch := range batches {
			plan, err := prepareVerification(scene, batch.Receivers, batch.Estimates, batch.Options, batch.Policy, "fixture")
			if err != nil {
				t.Fatal(err)
			}

			if err = appendVerificationGroup(&groups, index, plan, plan.Options); err != nil {
				t.Fatal(err)
			}
		}

		if len(groups) != 2 {
			t.Fatal("incompatible transport or independently reused receiver identities were combined", overlap)
		}
	}
}

func TestVerificationBatchPreservesQualificationDiagnostics(t *testing.T) {
	t.Parallel()
	scene, batches := verificationBatchesFixture(t)
	batches[0].Policy.RequireAllReceivers = true
	executor := &verificationSeedExecutor{}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	worker := &GPUWorker{
		ctx:      ctx,
		cancel:   cancel,
		executor: executor,
		device:   executor.Device(),
		options:  GPUWorkerOptions{MemoryBytes: 128 << 20},
	}

	separate := make([]VerificationReport, len(batches))
	for index, batch := range batches {
		report, err := worker.VerifyRNM(t.Context(), scene, batch.Receivers, batch.Estimates, batch.Options, batch.Policy)
		if index == 0 && !errors.Is(err, ErrVerification) || index != 0 && err != nil {
			t.Fatal(err)
		}

		separate[index] = report
	}

	combined, err := worker.VerifyRNMBatches(t.Context(), scene, batches)
	if !errors.Is(err, ErrVerification) {
		t.Fatal(err)
	}

	if !bytes.Equal(verificationBatchJSON(t, separate), verificationBatchJSON(t, combined)) {
		t.Fatal("qualification diagnostics or other page reports changed")
	}

	if executor.calls != 2 {
		t.Fatal("unqualified page was dispatched or valid page was skipped", executor.calls)
	}
}

func TestVerificationBatchInputErrorCannotBeHiddenByQualification(t *testing.T) {
	t.Parallel()
	scene, batches := verificationBatchesFixture(t)
	batches[0].Policy.RequireAllReceivers = true
	batches[1].Estimates[0].ReceiverID++
	executor := &verificationSeedExecutor{}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	worker := &GPUWorker{
		ctx:      ctx,
		cancel:   cancel,
		executor: executor,
		device:   executor.Device(),
		options:  GPUWorkerOptions{MemoryBytes: 128 << 20},
	}

	_, err := worker.VerifyRNMBatches(t.Context(), scene, batches)
	if !errors.Is(err, ErrInvalid) || errors.Is(err, ErrVerification) {
		t.Fatal("representation diagnostics hid an invalid later input", err)
	}

	if executor.calls != 0 {
		t.Fatal("invalid later page reached GPU")
	}
}
