//go:build linux

package native

import (
	"fmt"

	"github.com/gogpu/gputypes"
	"github.com/karty-game/ashura/internal/computedata"
	wgpu "github.com/karty-game/ashura/internal/webgpu"
	"github.com/karty-game/ashura/pkg/shader"
)

func (driver *computeBackend) prepareComputePipeline(source *computedata.Pipeline, limits wgpu.Limits) error {
	description := computedata.Description(source)
	if err := validateComputeLayout(description.Layout, limits); err != nil {
		return err
	}

	entries := make([]wgpu.BindGroupLayoutEntry, 0, len(description.Layout))
	for _, binding := range description.Layout {
		entry := wgpu.BindGroupLayoutEntry{Binding: binding.Binding, Visibility: gputypes.ShaderStageCompute}
		switch binding.Type {
		case shader.Uniform:
			entry.Buffer = &wgpu.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform, MinBindingSize: binding.MinSize}
		case shader.ReadOnlyStorage:
			entry.Buffer = &wgpu.BufferBindingLayout{Type: gputypes.BufferBindingTypeReadOnlyStorage, MinBindingSize: binding.MinSize}
		case shader.ReadWriteStorage:
			entry.Buffer = &wgpu.BufferBindingLayout{Type: gputypes.BufferBindingTypeStorage, MinBindingSize: binding.MinSize}
		case shader.StorageTexture:
			entry.StorageTexture = &wgpu.StorageTextureBindingLayout{
				Access:        gputypes.StorageTextureAccessWriteOnly,
				Format:        gputypes.TextureFormatRGBA8Unorm,
				ViewDimension: gputypes.TextureViewDimension2D,
			}
		}

		entries = append(entries, entry)
	}

	layout, err := driver.device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{Label: "compute group zero", Entries: entries})
	if err != nil {
		return fmt.Errorf("create compute layout: %w", err)
	}

	pipelineLayout, err := driver.device.CreatePipelineLayout(
		&wgpu.PipelineLayoutDescriptor{BindGroupLayouts: []*wgpu.BindGroupLayout{layout}},
	)
	if err != nil {
		layout.Release()

		return fmt.Errorf("create compute pipeline layout: %w", err)
	}
	defer pipelineLayout.Release()

	module, err := driver.device.CreateShaderModuleWGSL(description.Program.Source())
	if err != nil {
		layout.Release()

		return fmt.Errorf("create compute shader: %w", err)
	}
	defer module.Release()

	pipeline, err := driver.device.CreateComputePipeline(&wgpu.ComputePipelineDescriptor{
		Label:      description.Program.Name() + "::" + description.EntryPoint,
		Layout:     pipelineLayout,
		Module:     module,
		EntryPoint: description.EntryPoint,
	})
	if err != nil {
		layout.Release()

		return fmt.Errorf("create compute pipeline: %w", err)
	}

	driver.computePipelines[source] = gpuComputePipeline{pipeline: pipeline, layout: layout}

	return nil
}

func validateComputeLayout(layout shader.Layout, limits wgpu.Limits) error {
	var uniforms, storage, images uint32

	for _, binding := range layout {
		if binding.Binding >= limits.MaxBindingsPerBindGroup {
			return errComputeLimit
		}

		switch binding.Type {
		case shader.Uniform:
			uniforms++

			if binding.MinSize > limits.MaxUniformBufferBindingSize {
				return errComputeLimit
			}
		case shader.ReadOnlyStorage, shader.ReadWriteStorage:
			storage++

			if binding.MinSize > limits.MaxStorageBufferBindingSize {
				return errComputeLimit
			}
		case shader.StorageTexture:
			images++
		}
	}

	if uniforms > limits.MaxUniformBuffersPerShaderStage || storage > limits.MaxStorageBuffersPerShaderStage ||
		images > limits.MaxStorageTexturesPerShaderStage {
		return errComputeLimit
	}

	return nil
}
