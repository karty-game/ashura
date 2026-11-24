//go:build linux

package native

import (
	"errors"
	"fmt"

	"github.com/gogpu/gputypes"
	"github.com/karty-game/ashura/internal/bufferdata"
	"github.com/karty-game/ashura/internal/computedata"
	wgpu "github.com/karty-game/ashura/internal/webgpu"
)

type gpuComputePipeline struct {
	pipeline *wgpu.ComputePipeline
	layout   *wgpu.BindGroupLayout
}

var (
	errComputeLimit      = errors.New("compute resources exceed this device's limits")
	errComputeResource   = errors.New("compute resource was not prepared")
	errComputeValidation = errors.New("compute GPU validation failed")
)

// PrepareCompute creates immutable device resources before the first frame.
// Validation scopes are resolved only here, never during normal presentation.
func (driver *computeBackend) PrepareCompute(resources computedata.Resources) error {
	if driver.closed {
		return errDriverClosed
	}

	driver.computeBuffers = make(map[*bufferdata.Buffer]*wgpu.Buffer, len(resources.Buffers))
	driver.computePipelines = make(map[*computedata.Pipeline]gpuComputePipeline, len(resources.Pipelines))
	driver.computeGroups = make(map[*computedata.BindGroup]*wgpu.BindGroup, len(resources.BindGroups))

	limits := driver.device.Limits()
	for _, source := range resources.Buffers {
		if err := driver.prepareComputeBuffer(source, limits); err != nil {
			return err
		}
	}

	if len(resources.Pipelines) > 0 && limits.MaxComputeWorkgroupsPerDimension == 0 {
		return errComputeLimit
	}

	driver.computeWorkgroups = limits.MaxComputeWorkgroupsPerDimension

	for _, source := range resources.Pipelines {
		description := computedata.Description(source)

		label := description.Program.Name() + "::" + description.EntryPoint
		if err := driver.checkComputeSetup(label, func() error { return driver.prepareComputePipeline(source, limits) }); err != nil {
			return err
		}
	}

	for _, source := range resources.BindGroups {
		description := computedata.Description(computedata.PipelineFor(source))

		label := description.Program.Name() + "::" + description.EntryPoint + " bindings"
		if err := driver.checkComputeSetup(label, func() error { return driver.prepareComputeGroup(source) }); err != nil {
			return err
		}
	}

	return nil
}

func (driver *computeBackend) prepareComputeBuffer(source *bufferdata.Buffer, limits wgpu.Limits) error {
	maximum := limits.MaxStorageBufferBindingSize
	usage := gputypes.BufferUsageStorage | gputypes.BufferUsageCopyDst

	if bufferdata.Kind(source) == bufferdata.Uniform {
		maximum = limits.MaxUniformBufferBindingSize
		usage = gputypes.BufferUsageUniform | gputypes.BufferUsageCopyDst
	}

	if source.Size() > limits.MaxBufferSize || source.Size() > maximum {
		return fmt.Errorf("buffer %s: %w", source.Label(), errComputeLimit)
	}

	return driver.checkComputeSetup("buffer "+source.Label(), func() error {
		buffer, err := driver.device.CreateBuffer(&wgpu.BufferDescriptor{Label: source.Label(), Size: source.Size(), Usage: usage})
		if err != nil {
			return fmt.Errorf("create compute buffer: %w", err)
		}

		driver.computeBuffers[source] = buffer

		return nil
	})
}

func (driver *computeBackend) checkComputeSetup(label string, create func() error) error {
	driver.device.PushErrorScope(wgpu.ErrorFilterValidation)

	operationError := create()
	kind, message, popError := driver.device.PopErrorScopeAsync(driver.instance)

	if operationError != nil {
		return fmt.Errorf("compute %s: %w", label, operationError)
	}

	if popError != nil {
		return fmt.Errorf("validate compute %s: %w", label, popError)
	}

	if kind != wgpu.ErrorTypeNoError {
		return fmt.Errorf("compute %s: %w: %s", label, errComputeValidation, message)
	}

	return nil
}

func (driver *computeBackend) prepareComputeGroup(source *computedata.BindGroup) error {
	pipeline, exists := driver.computePipelines[computedata.PipelineFor(source)]
	if !exists {
		return errComputeResource
	}

	bindings := computedata.Bindings(source)

	entries := make([]wgpu.BindGroupEntry, 0, len(bindings))
	for _, binding := range bindings {
		entry := wgpu.BindGroupEntry{Binding: binding.Binding}
		if binding.Buffer != nil {
			entry.Buffer = driver.computeBuffers[binding.Buffer]
			if entry.Buffer == nil {
				return errComputeResource
			}

			entry.Size = binding.Buffer.Size()
		} else {
			view, err := driver.textureView(binding.Texture)
			if err != nil {
				return err
			}

			entry.TextureView = view
		}

		entries = append(entries, entry)
	}

	group, err := driver.device.CreateBindGroup(
		&wgpu.BindGroupDescriptor{Label: "compute bindings", Layout: pipeline.layout, Entries: entries},
	)
	if err != nil {
		return fmt.Errorf("create compute bindings: %w", err)
	}

	driver.computeGroups[source] = group

	return nil
}

func (driver *computeBackend) uploadComputeBuffers(uploads []bufferdata.Upload) error {
	for _, upload := range uploads {
		buffer := driver.computeBuffers[upload.Buffer]
		if buffer == nil {
			return errComputeResource
		}

		if err := driver.queue.WriteBuffer(buffer, upload.Offset, upload.Data); err != nil {
			return fmt.Errorf("write compute buffer %s: %w", upload.Buffer.Label(), err)
		}
	}

	return nil
}

func (driver *computeBackend) recordCompute(encoder *wgpu.CommandEncoder, commands []computedata.Dispatch) error {
	if len(commands) == 0 {
		return nil
	}

	pass, err := encoder.BeginComputePass(nil)
	if err != nil {
		return fmt.Errorf("begin compute pass: %w", err)
	}
	defer pass.Release()
	defer pass.End()

	for _, command := range commands {
		pipeline, exists := driver.computePipelines[command.Pipeline]

		group := driver.computeGroups[command.Group]
		if !exists || group == nil {
			return errComputeResource
		}

		if command.X > driver.computeWorkgroups || command.Y > driver.computeWorkgroups || command.Z > driver.computeWorkgroups {
			return errComputeLimit
		}

		pass.SetPipeline(pipeline.pipeline)
		pass.SetBindGroup(0, group, nil)
		pass.DispatchWorkgroups(command.X, command.Y, command.Z)
		driver.metrics.Dispatch()
	}

	return nil
}

// UnloadBuffer drops the device reference; already-submitted work retains its own.
func (driver *computeBackend) UnloadBuffer(source *bufferdata.Buffer) {
	if buffer := driver.computeBuffers[source]; buffer != nil {
		buffer.Release()
		delete(driver.computeBuffers, source)
	}
}

// UnloadComputePipeline drops a pipeline after its bind groups have been unloaded.
func (driver *computeBackend) UnloadComputePipeline(source *computedata.Pipeline) {
	if pipeline, exists := driver.computePipelines[source]; exists {
		pipeline.pipeline.Release()
		pipeline.layout.Release()
		delete(driver.computePipelines, source)
	}
}

// UnloadComputeBindings drops a cached group without blocking the GPU queue.
func (driver *computeBackend) UnloadComputeBindings(source *computedata.BindGroup) {
	if group := driver.computeGroups[source]; group != nil {
		group.Release()
		delete(driver.computeGroups, source)
	}
}

func (driver *computeBackend) Close() {
	for source := range driver.computeGroups {
		driver.UnloadComputeBindings(source)
	}

	for source := range driver.computePipelines {
		driver.UnloadComputePipeline(source)
	}

	for source := range driver.computeBuffers {
		driver.UnloadBuffer(source)
	}
}
