//go:build linux

package native

import (
	"fmt"

	"github.com/gogpu/gputypes"
	wgpu "github.com/karty-game/ashura/internal/webgpu"
)

func initializeGPU(driver *Driver) error {
	instance, err := wgpu.CreateInstance(nil)
	if err != nil {
		return fmt.Errorf("create WebGPU instance: %w", err)
	}

	driver.instance = instance

	surface, err := createSurface(instance, driver.window)
	if err != nil {
		return err
	}

	driver.surface = surface

	adapter, err := instance.RequestAdapter(&wgpu.RequestAdapterOptions{CompatibleSurface: surface})
	if err != nil {
		return fmt.Errorf("request WebGPU adapter: %w", err)
	}

	driver.adapter = adapter

	if err := initializeGPUDevice(driver); err != nil {
		return err
	}

	capabilities, err := surface.GetCapabilities(adapter)
	if err != nil {
		return fmt.Errorf("query surface capabilities: %w", err)
	}

	if len(capabilities.Formats) == 0 {
		return errNoSurfaceFormats
	}

	driver.format = capabilities.Formats[0]
	for _, format := range capabilities.Formats {
		if format == gputypes.TextureFormatBGRA8UnormSrgb || format == gputypes.TextureFormatRGBA8UnormSrgb {
			driver.format = format

			break
		}
	}

	return resize(driver)
}

// initializeGPUDevice enables prepared texture formats on the selected adapter.
// Keeping device setup independent of the surface also permits offscreen review.
func initializeGPUDevice(driver *Driver) error {
	device, err := driver.adapter.RequestDevice(&wgpu.DeviceDescriptor{RequiredFeatures: compressionFeatures(driver.adapter)})
	if err != nil {
		return fmt.Errorf("request WebGPU device: %w", err)
	}

	driver.device = device
	driver.compression = enabledCompression(device)
	driver.queue = device.Queue()

	if driver.queue == nil {
		return errQueueUnavailable
	}

	return nil
}
