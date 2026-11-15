//go:build js && wasm

package browser

import (
	"syscall/js"

	"github.com/karty-game/karty-webgpu/internal/bufferdata"
	"github.com/karty-game/karty-webgpu/internal/computedata"
	"github.com/karty-game/karty-webgpu/internal/platform"
)

// PrepareCompute admits work only when compute was explicitly registered.
func (driver *Driver) PrepareCompute(resources computedata.Resources) error {
	if len(resources.Buffers) == 0 && len(resources.Pipelines) == 0 && len(resources.BindGroups) == 0 {
		return nil
	}

	if driver.computeBridge == nil {
		return platform.ErrFeatureMissing
	}

	return driver.computeBridge.PrepareCompute(resources)
}

func (driver *Driver) uploadComputeBuffers(uploads []bufferdata.Upload) error {
	if len(uploads) == 0 {
		return nil
	}

	if driver.computeBridge == nil {
		return platform.ErrFeatureMissing
	}

	return driver.computeBridge.uploadComputeBuffers(uploads)
}

func (driver *Driver) recordCompute(encoder js.Value, commands []computedata.Dispatch) error {
	if len(commands) == 0 {
		return nil
	}

	if driver.computeBridge == nil {
		return platform.ErrFeatureMissing
	}

	return driver.computeBridge.recordCompute(encoder, commands)
}

func (driver *Driver) UnloadBuffer(source *bufferdata.Buffer) {
	if driver.computeBridge != nil {
		driver.computeBridge.UnloadBuffer(source)
	}
}

func (driver *Driver) UnloadComputePipeline(source *computedata.Pipeline) {
	if driver.computeBridge != nil {
		driver.computeBridge.UnloadComputePipeline(source)
	}
}

func (driver *Driver) UnloadComputeBindings(source *computedata.BindGroup) {
	if driver.computeBridge != nil {
		driver.computeBridge.UnloadComputeBindings(source)
	}
}

func (driver *Driver) releaseCompute() {
	if driver.computeBridge != nil {
		driver.computeBridge.Close()
		driver.computeBridge = nil
	}
}
