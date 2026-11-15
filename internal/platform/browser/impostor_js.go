//go:build js && wasm

package browser

import (
	"syscall/js"

	"github.com/karty-game/karty-webgpu/internal/impostordata"
	"github.com/karty-game/karty-webgpu/internal/render"
)

const (
	shaderCodeProperty = "code"
	gpuBufferProperty  = "buffer"
)

type gpuImpostor struct {
	buffer     js.Value
	references int
	owner      *Driver
	config     *impostordata.Document
}

func (driver *Driver) createImpostorPipeline(format string) error {
	shader := driver.device.Call(
		"createShaderModule",
		jsObject(map[string]any{labelProperty: "impostor.wgsl", shaderCodeProperty: render.ImpostorShader()}),
	)

	return driver.createPipelineVariant(format, "impostor.wgsl", false, false, true, shader)
}

func (driver *Driver) drawPipeline(draw render.DrawCall) js.Value {
	if draw.Impostor != nil {
		return driver.impostorPipeline
	}

	return driver.materialPipeline(draw.Source, draw.CullBackfaces)
}

// Immutable view tables are copied/uploaded once per resident asset, shared by
// matching material batches. Camera movement never touches this storage.
func (driver *Driver) acquireImpostor(config *impostordata.Document) *gpuImpostor {
	if cached := driver.impostorViews[config]; cached != nil {
		cached.references++

		return cached
	}

	if driver.impostorViews == nil {
		driver.impostorViews = make(map[*impostordata.Document]*gpuImpostor)
	}

	if driver.impostorSampler.IsUndefined() {
		driver.impostorSampler = driver.device.Call("createSampler", jsObject(map[string]any{
			addressModeUProperty: clampToEdge, addressModeVProperty: clampToEdge,
			magFilterProperty: linearFilter, minFilterProperty: linearFilter, mipmapFilterProperty: linearFilter,
		}))
	}

	buffer := driver.device.Call(
		"createBuffer",
		jsObject(map[string]any{sizeProperty: len(config.Uniform), usageProperty: uniformBufferUsage}),
	)
	data := js.Global().Get("Uint8Array").New(len(config.Uniform))
	js.CopyBytesToJS(data, config.Uniform)
	driver.queue.Call("writeBuffer", buffer, 0, data)
	value := &gpuImpostor{buffer: buffer, references: 1, owner: driver, config: config}
	driver.impostorViews[config] = value

	return value
}

func (value *gpuImpostor) release() {
	value.references--
	if value.references == 0 {
		value.buffer.Call("destroy")
		delete(value.owner.impostorViews, value.config)
	}
}
