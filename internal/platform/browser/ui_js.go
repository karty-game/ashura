//go:build js && wasm

//nolint:mnd,gosec // GPU ABI offsets/usages and nonnegative core-bounded draw counts.
package browser

import (
	"syscall/js"

	"github.com/karty-game/karty-webgpu/internal/uipaint"
)

type uiGPU struct {
	pipeline, uniform, buffer, group, array, sampler js.Value
	pages                                            map[*uipaint.Texture]uiPage
	capacity                                         int
	submission                                       uipaint.Submission
	passDescriptor, passAttachment                   js.Value
	revision                                         uint64
	width, height                                    float32
}

func (driver *browserUIBridge) prepareUI() error {
	if driver.ui == nil {
		return nil
	}

	module := driver.device.Call("createShaderModule", jsObject(map[string]any{"label": "UI MSDF and panels", "code": uipaint.Shader}))
	blend := jsObject(map[string]any{"operation": "add", "srcFactor": "one", "dstFactor": "one-minus-src-alpha"})

	pipeline, err := awaitPromise(driver.device.Call("createRenderPipelineAsync", jsObject(map[string]any{
		layoutProperty: "auto",
		"vertex": jsObject(
			map[string]any{
				"module":     module,
				"entryPoint": "vertexMain",
				uiVertexBuffers: []any{
					jsObject(
						map[string]any{
							"arrayStride": uipaint.Stride,
							"stepMode":    uiInstanceStep,
							"attributes": []any{
								jsObject(map[string]any{formatProperty: uiFloat4Format, "offset": 0, "shaderLocation": 0}),
								jsObject(map[string]any{formatProperty: uiFloat4Format, "offset": 64, "shaderLocation": 1}),
							},
						},
					),
				},
			},
		),
		"fragment": jsObject(
			map[string]any{
				"module":     module,
				"entryPoint": "fragmentMain",
				"targets": []any{
					jsObject(
						map[string]any{
							formatProperty: driver.colorFormat,
							"blend":        jsObject(map[string]any{"color": blend, "alpha": blend}),
						},
					),
				},
			},
		),
		"primitive": jsObject(map[string]any{"topology": "triangle-list"}),
	})))
	if err != nil {
		return err
	}

	gpu := &uiGPU{pipeline: pipeline, pages: make(map[*uipaint.Texture]uiPage)}
	gpu.passAttachment = jsObject(map[string]any{loadOpProperty: "load", storeOpProperty: uiStore})
	gpu.passDescriptor = jsObject(map[string]any{uiAttachments: []any{gpu.passAttachment}})
	gpu.uniform = driver.device.Call("createBuffer", jsObject(map[string]any{sizeProperty: 16, usageProperty: 0x48}))
	gpu.sampler = driver.device.Call(
		"createSampler",
		jsObject(
			map[string]any{
				"magFilter":    linearFilter,
				"minFilter":    linearFilter,
				"addressModeU": clampToEdge,
				"addressModeV": clampToEdge,
			},
		),
	)
	driver.uiGPU = gpu

	return nil
}

func (driver *browserUIBridge) updateUI(frame *uipaint.Frame) error {
	if frame == nil {
		return nil
	}

	gpu := driver.uiGPU
	if gpu == nil {
		return errDrawBuffer
	}

	if gpu.revision != frame.Revision {
		if err := driver.validateUIPages(frame); err != nil {
			return err
		}
	}

	frame = gpu.submission.Update(frame)

	if gpu.width != frame.Width || gpu.height != frame.Height {
		bytes := uipaint.Viewport(frame.Width, frame.Height, false)
		array := js.Global().Get("Uint8Array").New(16)
		js.CopyBytesToJS(array, bytes)
		driver.queue.Call("writeBuffer", gpu.uniform, 0, array)
		gpu.width, gpu.height = frame.Width, frame.Height
	}

	if gpu.revision == frame.Revision {
		return nil
	}

	if len(frame.Data) > 0 {
		driver.writeUIRecords(frame)
	}

	if err := driver.updateUIPages(frame); err != nil {
		return err
	}

	gpu.revision = frame.Revision

	return nil
}

func (driver *browserUIBridge) writeUIRecords(frame *uipaint.Frame) {
	gpu := driver.uiGPU

	full := gpu.revision != frame.Base
	if len(frame.Data) > gpu.capacity {
		if !gpu.buffer.IsUndefined() {
			gpu.buffer.Call("destroy")
		}

		gpu.capacity = nextCapacity(len(frame.Data))
		gpu.buffer = driver.device.Call(
			"createBuffer",
			jsObject(map[string]any{"label": "retained UI records", sizeProperty: gpu.capacity, usageProperty: 0xa8}),
		)
		gpu.array = js.Global().Get("Uint8Array").New(gpu.capacity)
		gpu.group = driver.device.Call(
			"createBindGroup",
			jsObject(map[string]any{layoutProperty: gpu.pipeline.Call("getBindGroupLayout", 0), "entries": []any{
				jsObject(map[string]any{bindingProperty: 0, resourceProperty: jsObject(map[string]any{gpuBufferProperty: gpu.uniform})}),
				jsObject(map[string]any{bindingProperty: 1, resourceProperty: jsObject(map[string]any{gpuBufferProperty: gpu.buffer})}),
			}}),
		)
		full = true
	}

	if full {
		js.CopyBytesToJS(gpu.array, frame.Data)
		driver.queue.Call("writeBuffer", gpu.buffer, 0, gpu.array, 0, len(frame.Data))
	} else {
		for _, change := range frame.Changes {
			js.CopyBytesToJS(gpu.array, frame.Data[change.Offset:change.Offset+change.Size])
			driver.queue.Call("writeBuffer", gpu.buffer, change.Offset, gpu.array, 0, change.Size)
		}
	}
}
func (driver *browserUIBridge) uploadUIPage(page *uipaint.Texture) (uiPage, error) {
	return driver.uploadPreparedPage(page, driver.uiGPU.pipeline, driver.uiGPU.sampler)
}

func (driver *browserUIBridge) validateUIPages(frame *uipaint.Frame) error {
	for _, batch := range frame.Batches {
		if batch.Count == 0 {
			continue
		}

		if _, exists := driver.uiGPU.pages[batch.Texture]; exists {
			continue
		}

		if _, err := driver.preparedPageFormat(batch.Texture); err != nil {
			return err
		}
	}

	return nil
}
func (driver *browserUIBridge) drawUI(encoder, view js.Value, frame *uipaint.Frame) {
	if frame == nil {
		return
	}

	gpu := driver.uiGPU

	frame = gpu.submission.Current()
	if len(frame.Data) == 0 {
		return
	}

	gpu.passAttachment.Set("view", view)
	pass := driver.startRenderPass(encoder, gpu.passDescriptor)
	pass.Call("setPipeline", gpu.pipeline)
	pass.Call("setVertexBuffer", 0, gpu.buffer)
	pass.Call("setBindGroup", 0, gpu.group)

	for _, batch := range frame.Batches {
		if batch.Count == 0 {
			continue
		}

		pass.Call("setBindGroup", 1, gpu.pages[batch.Texture].group)
		pass.Call("draw", 6, batch.Count, 0, batch.First)
		driver.metrics.DrawUI(6, uint64(batch.Count))
	}

	pass.Call("end")
}
func (driver *browserUIBridge) closeUI() {
	gpu := driver.uiGPU
	if gpu == nil {
		return
	}

	for _, page := range gpu.pages {
		page.texture.Call("destroy")
	}

	if !gpu.buffer.IsUndefined() {
		gpu.buffer.Call("destroy")
	}

	gpu.uniform.Call("destroy")

	driver.uiGPU = nil
}

func (driver *browserUIBridge) updateUIPages(frame *uipaint.Frame) error {
	gpu := driver.uiGPU

	for _, batch := range frame.Batches {
		if batch.Count == 0 {
			continue
		}

		if _, ok := gpu.pages[batch.Texture]; !ok {
			page, err := driver.uploadUIPage(batch.Texture)
			if err != nil {
				return err
			}

			gpu.pages[batch.Texture] = page
		}
	}

	for _, batch := range frame.Batches {
		if batch.Count > 0 {
			page := gpu.pages[batch.Texture]
			page.used = frame.Revision
			gpu.pages[batch.Texture] = page
		}
	}

	for source, page := range gpu.pages {
		if page.used != frame.Revision {
			page.texture.Call("destroy")
			delete(gpu.pages, source)
		}
	}

	return nil
}
