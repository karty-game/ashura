//go:build js && wasm

package browser

import (
	"fmt"
	"syscall/js"

	"github.com/karty-game/ashura/internal/bufferdata"
	"github.com/karty-game/ashura/internal/computedata"
	"github.com/karty-game/ashura/pkg/shader"
)

func (driver *computeBackend) validateComputeLimits(resources computedata.Resources) error {
	limits := driver.device.Get("limits")

	workgroups := limits.Get("maxComputeWorkgroupsPerDimension")
	if workgroups.Type() != js.TypeNumber || workgroups.Float() < 1 {
		return fmt.Errorf("%w: compute workgroups unavailable", errComputeLimit)
	}

	driver.computeWorkgroups = uint32(workgroups.Float())

	for _, buffer := range resources.Buffers {
		bindingLimit := "maxUniformBufferBindingSize"
		if bufferdata.Kind(buffer) == bufferdata.Storage {
			bindingLimit = "maxStorageBufferBindingSize"
		}

		size := float64(buffer.Size())
		if size > limits.Get("maxBufferSize").Float() || size > limits.Get(bindingLimit).Float() {
			return fmt.Errorf("%w: buffer %s size %d", errComputeLimit, buffer.Label(), buffer.Size())
		}
	}

	for _, pipeline := range resources.Pipelines {
		var uniforms, storage, images int

		description := computedata.Description(pipeline)
		for _, binding := range description.Layout {
			switch binding.Type {
			case shader.Uniform:
				uniforms++
			case shader.ReadOnlyStorage, shader.ReadWriteStorage:
				storage++
			case shader.StorageTexture:
				images++
			}
		}

		if uniforms > limits.Get("maxUniformBuffersPerShaderStage").Int() ||
			storage > limits.Get("maxStorageBuffersPerShaderStage").Int() ||
			images > limits.Get("maxStorageTexturesPerShaderStage").Int() {
			return fmt.Errorf("%w: bindings for %s entry %s", errComputeLimit, description.Program.Name(), description.EntryPoint)
		}
	}

	maximumDimension := limits.Get("maxTextureDimension2D").Int()

	for _, group := range resources.BindGroups {
		for _, binding := range computedata.Bindings(group) {
			if binding.Texture == nil {
				continue
			}

			width, height := binding.Texture.Size()
			if width > maximumDimension || height > maximumDimension {
				description := computedata.Description(computedata.PipelineFor(group))

				return fmt.Errorf(
					"%w: texture %dx%d for %s entry %s",
					errComputeLimit,
					width,
					height,
					description.Program.Name(),
					description.EntryPoint,
				)
			}
		}
	}

	return nil
}
