//go:build linux

package native

import (
	"errors"

	"github.com/karty-game/karty-webgpu/internal/bufferdata"
	"github.com/karty-game/karty-webgpu/internal/computedata"
	wgpu "github.com/karty-game/karty-webgpu/internal/webgpu"
)

// The core driver holds only these hooks. The concrete GPU implementation is
// installed by the explicitly selected compute feature.
type computeBackendInterface interface {
	PrepareCompute(resources computedata.Resources) error
	uploadComputeBuffers(uploads []bufferdata.Upload) error
	recordCompute(encoder *wgpu.CommandEncoder, commands []computedata.Dispatch) error
	submitComputeOnly(schedule []computedata.Dispatch) error
	UnloadBuffer(source *bufferdata.Buffer)
	UnloadComputePipeline(source *computedata.Pipeline)
	UnloadComputeBindings(source *computedata.BindGroup)
	Close()
}

type computeBackend struct {
	*Driver

	computeBuffers    map[*bufferdata.Buffer]*wgpu.Buffer
	computePipelines  map[*computedata.Pipeline]gpuComputePipeline
	computeGroups     map[*computedata.BindGroup]*wgpu.BindGroup
	computeWorkgroups uint32
}

var errComputeRegistration = errors.New("native compute registration requires an available driver without an existing compute backend")

// RegisterCompute installs compute support after the native device is ready.
// Resource preparation still occurs before the first application frame.
func RegisterCompute(host any) error {
	driver, ok := host.(*Driver)
	if !ok || driver == nil || driver.closed || driver.computeBridge != nil || driver.device == nil || driver.queue == nil {
		return errComputeRegistration
	}

	driver.computeBridge = &computeBackend{Driver: driver}

	return nil
}
