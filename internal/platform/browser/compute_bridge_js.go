//go:build js && wasm

package browser

import (
	"errors"
	"syscall/js"

	"github.com/karty-game/ashura/internal/bufferdata"
	"github.com/karty-game/ashura/internal/computedata"
	"github.com/karty-game/ashura/pkg/shader"
)

// The core driver holds only these hooks. The concrete GPU implementation is
// installed by the explicitly selected compute feature.
type computeBackendInterface interface {
	PrepareCompute(resources computedata.Resources) error
	uploadComputeBuffers(uploads []bufferdata.Upload) error
	recordCompute(encoder js.Value, commands []computedata.Dispatch) error
	UnloadBuffer(source *bufferdata.Buffer)
	UnloadComputePipeline(source *computedata.Pipeline)
	UnloadComputeBindings(source *computedata.BindGroup)
	Close()
}

type computeBackend struct {
	*Driver

	computeBuffers    map[*bufferdata.Buffer]gpuComputeBuffer
	computePipelines  map[*computedata.Pipeline]gpuComputePipeline
	computeBindings   map[*computedata.BindGroup]js.Value
	computeModules    map[*shader.Module]js.Value
	computeWorkgroups uint32
}

var errComputeRegistration = errors.New("browser compute registration requires an available driver without an existing compute backend")

// RegisterCompute installs compute support after the browser device is ready.
// Resource preparation still occurs before the first application frame.
func RegisterCompute(host any) error {
	driver, ok := host.(*Driver)
	if !ok || driver == nil || driver.closed || driver.running || driver.computeBridge != nil ||
		driver.device.IsUndefined() || driver.device.IsNull() || driver.queue.IsUndefined() || driver.queue.IsNull() {
		return errComputeRegistration
	}

	driver.computeBridge = &computeBackend{Driver: driver}

	return nil
}
