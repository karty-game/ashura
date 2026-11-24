//go:build !js

package lightmapper

import (
	"context"
	"sync"
	"sync/atomic"

	wgpu "github.com/karty-game/ashura/internal/webgpu"
)

// nativeGPUExecutor owns an offline headless device through the rendering binding.
// One serialized owner controls every native handle.
type nativeGPUExecutor struct {
	mu sync.Mutex
	//nolint:containedctx // One cancelable build-scoped device lifetime.
	ctx             context.Context
	cancel          context.CancelFunc
	device          GPUDevice
	limits          wgpu.Limits
	instance        *wgpu.Instance
	adapter         *wgpu.Adapter
	gpu             *wgpu.Device
	queue           *wgpu.Queue
	shader          *wgpu.ShaderModule
	layouts         [3]*wgpu.BindGroupLayout
	pipelineLayouts [3]*wgpu.PipelineLayout
	stages          [stageCount]*wgpu.ComputePipeline
	scene           *wgpu.Buffer
	traceRays       bool
	traceCounts     GPUTraceCounts
	memoryBytes     uint64
	residentKey     string
	residentBytes   uint64
	uploads         atomic.Uint64
	closed          atomic.Bool
	closeOnce       sync.Once
}

func newNativeGPUExecutor(ctx context.Context, options GPUWorkerOptions) (*nativeGPUExecutor, error) {
	source, err := nativeGPUShaderSource()
	if err != nil {
		return nil, err
	}

	return newNativeGPUExecutorSource(ctx, options, string(source))
}

func newNativeGPUExecutorSource(ctx context.Context, options GPUWorkerOptions, source string) (*nativeGPUExecutor, error) {
	if ctx == nil || options.MemoryBytes == 0 {
		return nil, ErrInvalid
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	life, cancel := context.WithCancel(ctx)

	executor := &nativeGPUExecutor{ctx: life, cancel: cancel, memoryBytes: options.MemoryBytes, traceRays: options.TraceRays}
	if err := executor.openDevice(options, source); err != nil {
		_ = executor.Close()

		return nil, err
	}

	if err := life.Err(); err != nil {
		_ = executor.Close()

		return nil, err
	}

	return executor, nil
}
func (executor *nativeGPUExecutor) Device() GPUDevice    { return executor.device }
func (executor *nativeGPUExecutor) SceneUploads() uint64 { return executor.uploads.Load() }
func (executor *nativeGPUExecutor) TraceRayCounts() GPUTraceCounts {
	executor.mu.Lock()
	defer executor.mu.Unlock()

	return executor.traceCounts
}
func (executor *nativeGPUExecutor) Close() error {
	if executor == nil {
		return nil
	}

	executor.closeOnce.Do(func() {
		executor.closed.Store(true)
		executor.cancel()
		executor.mu.Lock()
		defer executor.mu.Unlock()

		releaseNativeBuffer(executor.scene)

		for _, pipeline := range executor.stages {
			if pipeline != nil {
				pipeline.Release()
			}
		}

		for _, layout := range executor.pipelineLayouts {
			if layout != nil {
				layout.Release()
			}
		}

		for _, layout := range executor.layouts {
			if layout != nil {
				layout.Release()
			}
		}

		if executor.shader != nil {
			executor.shader.Release()
		}

		if executor.queue != nil {
			executor.queue.Release()
		}

		if executor.gpu != nil {
			executor.gpu.Release()
		}

		if executor.adapter != nil {
			executor.adapter.Release()
		}

		if executor.instance != nil {
			executor.instance.Release()
		}
	})

	return nil
}
func releaseNativeBuffer(buffer *wgpu.Buffer) {
	if buffer != nil {
		buffer.Destroy()
		buffer.Release()
	}
}
