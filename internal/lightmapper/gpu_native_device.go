//go:build !js

//nolint:mnd // Buffer layouts, flag words and dispatch widths mirror the production WGSL ABI.
package lightmapper

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/gogpu/gputypes"
	wgpu "github.com/karty-game/ashura/internal/webgpu"
)

const nativeIdentityPrefix = "ashura-webgpu/abi29-r1/scene5/transport4 "

func (executor *nativeGPUExecutor) openDevice(options GPUWorkerOptions, source string) error {
	identity, err := executor.openAdapter(options.SoftwareForCI)
	if err != nil {
		return err
	}

	required := executor.adapter.Limits()
	if required.MaxStorageBuffersPerShaderStage < 4 {
		return ErrUnsupported
	}

	required.MaxStorageBuffersPerShaderStage = 4
	if err = executor.requestDevice(&required); err != nil {
		return err
	}

	executor.limits = executor.gpu.Limits()
	executor.device = GPUDevice{Adapter: identity, SoftwareForCI: options.SoftwareForCI,
		MaxBufferSize: executor.limits.MaxBufferSize, MaxStorageBufferBindingSize: executor.limits.MaxStorageBufferBindingSize,
		MaxStorageBuffersPerShaderStage: executor.limits.MaxStorageBuffersPerShaderStage}

	executor.queue = executor.gpu.Queue()
	if executor.queue == nil {
		return errGPUExecution
	}

	if err = executor.ctx.Err(); err != nil {
		return err
	}

	return executor.compile(source)
}

// DetectGPUBackend identifies the adapter selected by the offline worker
// without requesting a device or compiling any shaders.
func DetectGPUBackend(ctx context.Context, softwareForCI bool) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	life, cancel := context.WithCancel(ctx)

	probe := nativeGPUExecutor{ctx: life, cancel: cancel}
	defer func() { _ = probe.Close() }()

	identity, err := probe.openAdapter(softwareForCI)
	if err != nil {
		return "", err
	}

	return identity, life.Err()
}

func (executor *nativeGPUExecutor) openAdapter(softwareForCI bool) (string, error) {
	var err error

	executor.instance, err = wgpu.CreateInstance(nil)
	if err != nil {
		return "", fmt.Errorf("create offline WebGPU instance: %w", err)
	}

	executor.adapter, err = executor.instance.RequestAdapter(&wgpu.RequestAdapterOptions{
		PowerPreference: gputypes.PowerPreferenceHighPerformance, ForceFallbackAdapter: softwareForCI,
	})
	if err != nil {
		return "", fmt.Errorf("request offline WebGPU adapter: %w", err)
	}

	info, err := executor.adapter.Info()
	if err != nil {
		return "", err
	}

	identity, err := nativeAdapterIdentity(info, softwareForCI)
	if err != nil {
		return "", err
	}

	return nativeIdentityPrefix + identity, nil
}

func (executor *nativeGPUExecutor) requestDevice(required *wgpu.Limits) error {
	// The binding's synchronous request needs instance event processing. Join it
	// before releasing native handles, including when the build was canceled.
	done := make(chan struct{})

	var err error
	go func() {
		executor.gpu, err = executor.adapter.RequestDevice(
			&wgpu.DeviceDescriptor{Label: "Ashura offline lightmapper", RequiredLimits: required},
		)

		close(done)
	}()

	for {
		select {
		case <-done:
			if err != nil {
				return fmt.Errorf("request offline WebGPU device: %w", err)
			}

			return executor.ctx.Err()
		default:
			executor.instance.ProcessEvents()
			runtime.Gosched()
		}
	}
}

func nativeAdapterIdentity(info *wgpu.AdapterInfoGo, softwareForCI bool) (string, error) {
	if info == nil {
		return "", ErrInvalid
	}

	identity := fmt.Sprintf("%s %s %s backend=%d type=%d vendorID=%04x deviceID=%04x",
		info.Vendor, info.Device, info.Description, info.BackendType, info.AdapterType, info.VendorID, info.DeviceID)
	hardware := info.AdapterType == wgpu.AdapterTypeDiscreteGPU || info.AdapterType == wgpu.AdapterTypeIntegratedGPU

	native := info.BackendType == wgpu.BackendTypeVulkan || info.BackendType == wgpu.BackendTypeMetal ||
		info.BackendType == wgpu.BackendTypeD3D12
	if softwareForCI {
		if info.AdapterType != wgpu.AdapterTypeCPU || info.BackendType != wgpu.BackendTypeVulkan {
			return "", ErrUnsupported
		}

		return identity + " mode=ci-software", nil
	}

	if !hardware || !native {
		requirement := "offline lightmap baking requires a hardware Vulkan, Metal or D3D12 adapter"
		if runtime.GOOS == "linux" {
			requirement = "offline lightmap baking requires a hardware Vulkan adapter; check the system Vulkan loader and GPU Vulkan driver (ICD) installation"
		}

		return "", fmt.Errorf("unsupported bake adapter %s; %s: %w", identity, requirement, ErrUnsupported)
	}

	for _, name := range []string{"software", "swiftshader", "llvmpipe", "softpipe", "lavapipe", "basic render driver"} {
		if strings.Contains(strings.ToLower(identity), name) {
			return "", ErrUnsupported
		}
	}

	return identity, nil
}

// Nonblocking polls keep cancellation observable. Unmap aborts a pending map;
// drain its callback before releasing the buffer and device.
func (executor *nativeGPUExecutor) mapReadback(ctx context.Context, buffer *wgpu.Buffer, size uint64) error {
	pending, err := buffer.MapAsync(wgpu.MapModeRead, 0, size)
	if err != nil {
		return err
	}
	defer pending.Release()

	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()

	var canceled error

	for {
		executor.gpu.Poll(false)
		executor.instance.ProcessEvents()

		if ready, mapErr := pending.Status(); ready {
			if canceled != nil {
				return canceled
			}

			return mapErr
		}

		if canceled == nil && ctx.Err() != nil {
			canceled = ctx.Err()
			_ = buffer.Unmap()
		}

		<-ticker.C
	}
}
