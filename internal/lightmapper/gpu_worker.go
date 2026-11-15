//go:build !js

package lightmapper

import (
	"context"
	"sync"
)

// GPUWorkerOptions configures the in-process offline compute device and its
// complete live allocation ceiling. No browser or helper process is launched.
type GPUWorkerOptions struct {
	MemoryBytes uint64
	// TraceRays enables offline integer ray diagnostics; disabled production pipelines compile them out.
	TraceRays bool
	// SoftwareForCI explicitly selects a native Vulkan CPU adapter for functional
	// CI only. The default requires hardware; software results are not GPU timing
	// or local visual-review evidence.
	SoftwareForCI bool
}

// GPUTraceCounts records successful trace queries, including all continuation
// and shadow rays. ShadowRays is a subset of Rays, not an additional count.
type GPUTraceCounts struct {
	Rays       uint64 `json:"rays"`
	ShadowRays uint64 `json:"shadowRays"`
}

// GPUDevice records actual native hardware identity and negotiated bounds.
type GPUDevice struct {
	Adapter                         string `json:"adapter"`
	SoftwareForCI                   bool   `json:"softwareForCi,omitempty"`
	MaxBufferSize                   uint64 `json:"maxBufferSize"`
	MaxStorageBufferBindingSize     uint64 `json:"maxStorageBufferBindingSize"`
	MaxStorageBuffersPerShaderStage uint32 `json:"maxStorageBuffersPerShaderStage"`
}

// GPUScene is checked immutable build data, privately owned by its creator.
type GPUScene struct {
	Key  string
	Data []byte
}

// GPUTile holds surface records independently of final atlas dimensions.
type GPUTile struct {
	Data       []byte
	Count      uint32
	Options    Options
	Mode       string
	ProbeCount uint32
}

type gpuExecutor interface {
	Device() GPUDevice
	Run(ctx context.Context, scene GPUScene, tile GPUTile) ([]byte, error)
	SceneUploads() uint64
	TraceRayCounts() GPUTraceCounts
	Close() error
}

// GPUWorker owns one native worker generation. Complete jobs are admitted before
// submission; cancellation or an execution failure invalidates that generation.
type GPUWorker struct {
	mu      sync.Mutex
	busy    sync.Mutex
	device  GPUDevice
	options GPUWorkerOptions
	//nolint:containedctx // One cancelable native worker generation spans independent build jobs.
	ctx         context.Context
	cancel      context.CancelFunc
	executor    gpuExecutor
	failure     error
	packedScene GPUScene
	closeOnce   sync.Once
	closeError  error
}

// NewGPUWorker opens an in-process headless GPU compute device. It does
// not create a window, surface, HTTP listener or browser process.
func NewGPUWorker(ctx context.Context, options GPUWorkerOptions) (*GPUWorker, error) {
	if ctx == nil || options.MemoryBytes == 0 {
		return nil, ErrInvalid
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	life, cancel := context.WithCancel(ctx)

	executor, err := newNativeGPUExecutor(life, options)
	if err != nil {
		cancel()

		return nil, err
	}

	if err = life.Err(); err != nil {
		cancel()

		_ = executor.Close()

		return nil, err
	}

	worker := &GPUWorker{options: options, ctx: life, cancel: cancel, executor: executor, device: executor.Device()}
	go func() { <-life.Done(); _ = worker.Close() }()

	return worker, nil
}

// Device returns immutable admitted native limits and hardware provenance.
func (w *GPUWorker) Device() GPUDevice {
	if w == nil {
		return GPUDevice{}
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	return w.device
}

// Run validates the complete batch before invoking the native executor. The
// 56-word linear output and independent verification layouts are unchanged.
func (w *GPUWorker) Run(ctx context.Context, scene GPUScene, tile GPUTile) ([]byte, error) {
	if w == nil || ctx == nil {
		return nil, ErrInvalid
	}

	if !w.busy.TryLock() {
		return nil, errGPUWorkerBusy
	}
	defer w.busy.Unlock()

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	w.mu.Lock()
	failure, executor := w.failure, w.executor
	w.mu.Unlock()

	if failure != nil {
		return nil, failure
	}

	if err := w.ctx.Err(); err != nil {
		return nil, err
	}

	if err := w.admit(scene, tile); err != nil {
		return nil, err
	}

	expected, err := gpuExpectedResultBytes(tile)
	if err != nil {
		return nil, err
	}

	data, err := executor.Run(ctx, scene, tile)
	if err != nil {
		if canceled := ctx.Err(); canceled != nil {
			err = canceled
		} else if canceled = w.ctx.Err(); canceled != nil {
			err = canceled
		}
	}

	if err == nil && int64(len(data)) != expected {
		err = ErrInvalid
	}

	if err != nil {
		w.fail(err)
		_ = w.Close()

		return nil, err
	}

	return data, nil
}

// Close cancels this executor and releases its owned native resources.
func (w *GPUWorker) Close() error {
	if w == nil {
		return nil
	}

	w.closeOnce.Do(func() {
		w.mu.Lock()
		if w.failure == nil {
			w.failure = context.Canceled
		}

		executor, cancel := w.executor, w.cancel
		w.mu.Unlock()

		if cancel != nil {
			cancel()
		}

		if executor != nil {
			w.closeError = executor.Close()
		}
	})

	return w.closeError
}

// SceneUploads reports completed scene transfers to the native worker.
func (w *GPUWorker) SceneUploads() uint64 {
	if w == nil {
		return 0
	}

	w.mu.Lock()
	executor := w.executor
	w.mu.Unlock()

	if executor == nil {
		return 0
	}

	return executor.SceneUploads()
}

// TraceRayCounts returns cumulative successful-job counters when TraceRays is enabled.
func (w *GPUWorker) TraceRayCounts() GPUTraceCounts {
	if w == nil {
		return GPUTraceCounts{}
	}

	w.mu.Lock()
	executor := w.executor
	w.mu.Unlock()

	if executor == nil {
		return GPUTraceCounts{}
	}

	return executor.TraceRayCounts()
}

func gpuPathRecordWords(tile GPUTile) uint64 {
	if tile.Mode == gpuModeVerify {
		return uint64(gpuPathWords) + uint64(tile.ProbeCount)*3
	}

	return gpuPathWords
}
func gpuOutputRecordWords(tile GPUTile) uint64 {
	if tile.Mode == gpuModeVerify {
		return uint64(tile.ProbeCount)*gpuVerificationMomentWords + gpuVerificationStatusWords
	}

	return gpuResultWords
}

func (w *GPUWorker) admit(scene GPUScene, tile GPUTile) error {
	device := w.Device()

	receiverCount := uint64(tile.Count)
	if tile.Mode == gpuModeVerify && (tile.ProbeCount == 0 || uint64(tile.ProbeCount) > device.MaxStorageBufferBindingSize/36) {
		return ErrBounds
	}

	if err := gpuValidatePackets(scene, tile); err != nil {
		return err
	}

	// Scene/receiver, 128-word path records and 34-word statistics are all live.
	sizes := []uint64{
		uint64(len(scene.Data)),
		uint64(len(tile.Data)),
		gpuPathHeaderBytes + receiverCount*(gpuPathRecordWords(tile)+gpuPathQueueCount)*gpuWordBytes,
		receiverCount * gpuOutputRecordWords(tile) * gpuWordBytes,
	}
	if tile.Mode == gpuModeReconstruct {
		sizes[2] = gpuPathMinimumBytes
	}

	total := uint64(64 + 12)

	for _, size := range sizes {
		if size > device.MaxBufferSize || size > device.MaxStorageBufferBindingSize {
			return ErrBounds
		}

		total += size
	}
	// Readback and host staging are accounted separately from GPU bindings.
	readback := receiverCount * gpuOutputRecordWords(tile) * gpuWordBytes
	if w.options.TraceRays && tile.Mode != gpuModeReconstruct {
		readback += receiverCount*gpuTraceCounterBytes + gpuWordBytes
	}

	if readback > device.MaxBufferSize {
		return ErrBounds
	}

	total += readback
	if total > w.options.MemoryBytes || device.MaxStorageBuffersPerShaderStage < 4 {
		return ErrBounds
	}

	return nil
}

func (w *GPUWorker) fail(err error) {
	w.mu.Lock()
	if w.failure == nil {
		w.failure = err
	}
	w.mu.Unlock()
}
