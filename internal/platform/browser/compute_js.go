//go:build js && wasm

package browser

import (
	"errors"
	"fmt"
	"syscall/js"

	"github.com/karty-game/ashura/internal/bufferdata"
	"github.com/karty-game/ashura/internal/computedata"
	"github.com/karty-game/ashura/pkg/shader"
)

const (
	computeShaderVisibility = 0x04
	storageBufferUsage      = 0x88
)

var (
	errComputeValidation = errors.New("browser compute validation failed")
	errComputeResource   = errors.New("browser compute resource is unavailable")
	errComputeLimit      = errors.New("browser compute resource exceeds device limits")
)

type gpuComputeBuffer struct{ buffer, array js.Value }
type gpuComputePipeline struct{ pipeline, layout js.Value }

// PrepareCompute creates immutable pipeline/group state before animation starts.
// Validation scopes and pipeline promise waits never occur in normal frames.
func (driver *computeBackend) PrepareCompute(resources computedata.Resources) error {
	if err := driver.deviceFailure(); err != nil {
		return err
	}

	if len(resources.Buffers) == 0 && len(resources.Pipelines) == 0 && len(resources.BindGroups) == 0 {
		return nil
	}

	if err := driver.validateComputeLimits(resources); err != nil {
		return err
	}

	driver.computeBuffers = make(map[*bufferdata.Buffer]gpuComputeBuffer, len(resources.Buffers))
	driver.computePipelines = make(map[*computedata.Pipeline]gpuComputePipeline, len(resources.Pipelines))
	driver.computeBindings = make(map[*computedata.BindGroup]js.Value, len(resources.BindGroups))
	driver.computeModules = make(map[*shader.Module]js.Value)
	driver.device.Call("pushErrorScope", "validation")
	prepareErr := driver.prepareComputeResources(resources)

	validation, scopeErr := awaitPromise(driver.device.Call("popErrorScope"))
	if scopeErr != nil {
		return fmt.Errorf("validate compute startup: %w", scopeErr)
	}

	if prepareErr != nil {
		if !validation.IsNull() && !validation.IsUndefined() {
			return fmt.Errorf("%w: %s", prepareErr, validation.Get("message").String())
		}

		return prepareErr
	}

	if !validation.IsNull() && !validation.IsUndefined() {
		return fmt.Errorf("%w: %s", errComputeValidation, validation.Get("message").String())
	}

	return nil
}

func (driver *computeBackend) prepareComputeResources(resources computedata.Resources) error {
	for _, source := range resources.Buffers {
		usage := uniformBufferUsage
		if bufferdata.Kind(source) == bufferdata.Storage {
			usage = storageBufferUsage
		}

		buffer := driver.device.Call("createBuffer", jsObject(map[string]any{
			labelProperty: source.Label(), sizeProperty: float64(source.Size()), usageProperty: usage,
		}))
		driver.computeBuffers[source] = gpuComputeBuffer{buffer: buffer, array: js.Global().Get("Uint8Array").New(float64(source.Size()))}
	}

	for _, source := range resources.Pipelines {
		if err := driver.createComputePipeline(source); err != nil {
			return err
		}
	}

	for _, source := range resources.BindGroups {
		if err := driver.createComputeBindings(source); err != nil {
			return err
		}
	}

	return nil
}

func (driver *computeBackend) createComputePipeline(source *computedata.Pipeline) error {
	description := computedata.Description(source)

	module, ok := driver.computeModules[description.Program]
	if !ok {
		module = driver.device.Call("createShaderModule", jsObject(map[string]any{
			labelProperty: description.Program.Name(), shaderCodeProperty: description.Program.Source(),
		}))
		driver.computeModules[description.Program] = module
	}

	entries := js.Global().Get("Array").New(len(description.Layout))
	for index, binding := range description.Layout {
		entries.SetIndex(index, computeLayoutEntry(binding))
	}

	layout := driver.device.Call("createBindGroupLayout", jsObject(map[string]any{entriesProperty: entries}))
	layouts := js.Global().Get("Array").New(1)
	layouts.SetIndex(0, layout)
	pipelineLayout := driver.device.Call("createPipelineLayout", jsObject(map[string]any{"bindGroupLayouts": layouts}))

	pipeline, err := awaitPromise(driver.device.Call("createComputePipelineAsync", jsObject(map[string]any{
		labelProperty:  description.Program.Name() + ":" + description.EntryPoint,
		layoutProperty: pipelineLayout,
		"compute":      jsObject(map[string]any{moduleProperty: module, entryPointProperty: description.EntryPoint}),
	})))
	if err != nil {
		return fmt.Errorf("compile compute %s entry %s: %w", description.Program.Name(), description.EntryPoint, err)
	}

	driver.computePipelines[source] = gpuComputePipeline{pipeline: pipeline, layout: layout}

	return nil
}

func computeLayoutEntry(binding shader.BindingLayout) js.Value {
	entry := map[string]any{bindingProperty: binding.Binding, "visibility": computeShaderVisibility}
	switch binding.Type {
	case shader.Uniform, shader.ReadOnlyStorage, shader.ReadWriteStorage:
		kind := "uniform"
		if binding.Type == shader.ReadOnlyStorage {
			kind = "read-only-storage"
		}

		if binding.Type == shader.ReadWriteStorage {
			kind = "storage"
		}

		entry["buffer"] = jsObject(map[string]any{"type": kind, "minBindingSize": float64(binding.MinSize)})
	case shader.StorageTexture:
		entry["storageTexture"] = jsObject(map[string]any{"access": "write-only", formatProperty: "rgba8unorm", "viewDimension": "2d"})
	}

	return jsObject(entry)
}

func (driver *computeBackend) createComputeBindings(source *computedata.BindGroup) error {
	pipeline, ok := driver.computePipelines[computedata.PipelineFor(source)]
	if !ok {
		return errComputeResource
	}

	bindings := computedata.Bindings(source)

	entries := js.Global().Get("Array").New(len(bindings))
	for index, binding := range bindings {
		var resource js.Value

		if binding.Buffer != nil {
			buffer, ok := driver.computeBuffers[binding.Buffer]
			if !ok {
				return errComputeResource
			}

			resource = jsObject(map[string]any{"buffer": buffer.buffer, sizeProperty: float64(binding.Buffer.Size())})
		} else {
			view, err := driver.textureView(binding.Texture)
			if err != nil {
				return err
			}

			resource = view
		}

		entries.SetIndex(index, jsObject(map[string]any{bindingProperty: binding.Binding, resourceProperty: resource}))
	}

	description := computedata.Description(computedata.PipelineFor(source))
	driver.computeBindings[source] = driver.device.Call("createBindGroup", jsObject(map[string]any{
		labelProperty: description.Program.Name() + ":" + description.EntryPoint, layoutProperty: pipeline.layout, entriesProperty: entries,
	}))

	return nil
}
