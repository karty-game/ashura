//go:build js && wasm

package browser

import (
	"syscall/js"

	"github.com/karty-game/ashura/internal/spritepaint"
)

type spriteGPU struct {
	pipeline, uniform, buffer, group, array, sampler js.Value
	passDescriptor, passAttachment                   js.Value
	pages                                            map[*spritepaint.Texture]uiPage
	batches                                          []spritepaint.Batch
	capacity                                         int
	revision, batchesRevision                        uint64
	width, height                                    float32
}

func (driver *spriteFeature) updateSprites(frame *spritepaint.Frame) error {
	if frame == nil {
		if driver.sprites != nil {
			driver.sprites.batches = nil
		}

		return nil
	}

	if driver.sprites == nil {
		return errSpriteGPU
	}

	gpu := driver.sprites
	if gpu.batchesRevision != frame.BatchesRevision {
		if err := driver.validateSpritePages(frame); err != nil {
			return err
		}
	}

	if gpu.width != frame.Width || gpu.height != frame.Height {
		linear := driver.colorFormat == "rgba8unorm-srgb" || driver.colorFormat == "bgra8unorm-srgb"
		bytes := spritepaint.Viewport(frame.Width, frame.Height, linear)
		array := js.Global().Get("Uint8Array").New(len(bytes))
		js.CopyBytesToJS(array, bytes)
		driver.queue.Call("writeBuffer", gpu.uniform, 0, array)
		gpu.width, gpu.height = frame.Width, frame.Height
	}

	if gpu.revision == frame.Revision {
		gpu.batches = frame.Batches

		return nil
	}

	if err := driver.writeSpriteRecords(frame); err != nil {
		return err
	}

	if gpu.batchesRevision != frame.BatchesRevision {
		if err := driver.updateSpritePages(frame); err != nil {
			return err
		}

		gpu.batchesRevision = frame.BatchesRevision
	}

	gpu.batches, gpu.revision = frame.Batches, frame.Revision

	return nil
}

func (driver *spriteFeature) writeSpriteRecords(frame *spritepaint.Frame) error {
	if len(frame.Data) == 0 {
		return nil
	}

	gpu := driver.sprites

	full := gpu.revision != frame.Base
	if len(frame.Data) > gpu.capacity {
		if err := driver.growSpriteBuffer(len(frame.Data)); err != nil {
			return err
		}

		full = true
	}

	if full {
		js.CopyBytesToJS(gpu.array, frame.Data)
		driver.queue.Call("writeBuffer", gpu.buffer, 0, gpu.array, 0, len(frame.Data))

		return nil
	}

	for _, change := range frame.Changes {
		js.CopyBytesToJS(gpu.array, frame.Data[change.Offset:change.Offset+change.Size])
		driver.queue.Call("writeBuffer", gpu.buffer, change.Offset, gpu.array, 0, change.Size)
	}

	return nil
}

func (driver *spriteFeature) growSpriteBuffer(size int) error {
	gpu := driver.sprites
	capacity := nextCapacity(size)

	limits := driver.device.Get("limits")
	if capacity > limits.Get("maxBufferSize").Int() || capacity > limits.Get("maxStorageBufferBindingSize").Int() {
		return errSpriteLimit
	}

	finishScope := driver.beginSpriteResources()
	defer finishScope()

	buffer := driver.device.Call("createBuffer", jsObject(map[string]any{
		labelProperty: "retained Screen2D records", sizeProperty: capacity, usageProperty: spriteStorageUsage,
	}))

	group := driver.device.Call("createBindGroup", jsObject(map[string]any{
		layoutProperty: gpu.pipeline.Call("getBindGroupLayout", 0), entriesProperty: []any{
			jsObject(map[string]any{bindingProperty: 0, resourceProperty: jsObject(map[string]any{gpuBufferProperty: gpu.uniform})}),
			jsObject(map[string]any{bindingProperty: 1, resourceProperty: jsObject(map[string]any{gpuBufferProperty: buffer})}),
		},
	}))
	if !gpu.buffer.IsUndefined() {
		gpu.buffer.Call("destroy")
	}

	gpu.buffer, gpu.group, gpu.capacity = buffer, group, capacity
	gpu.array = js.Global().Get("Uint8Array").New(capacity)

	return nil
}

func (driver *spriteFeature) drawSprites(encoder, view js.Value) {
	gpu := driver.sprites
	if gpu == nil || len(gpu.batches) == 0 {
		return
	}

	gpu.passAttachment.Set(viewProperty, view)
	pass := driver.startRenderPass(encoder, gpu.passDescriptor)
	pass.Call("setPipeline", gpu.pipeline)
	pass.Call("setVertexBuffer", 0, gpu.buffer)
	pass.Call("setBindGroup", 0, gpu.group)

	for _, batch := range gpu.batches {
		if batch.Count == 0 || batch.Clip[2] == 0 || batch.Clip[3] == 0 {
			continue
		}

		pass.Call("setScissorRect", batch.Clip[0], batch.Clip[1], batch.Clip[2], batch.Clip[3])
		pass.Call("setBindGroup", 1, gpu.pages[batch.Texture].group)
		pass.Call("draw", spriteVertices, batch.Count, 0, batch.First)
		driver.metrics.DrawSprites(
			spriteVertices,
			uint64(batch.Count), //nolint:gosec // Batch counts are nonnegative admitted sprite counts.
		)
	}

	pass.Call("end")
	gpu.passAttachment.Set(viewProperty, js.Undefined())
}

func (driver *spriteFeature) closeSprites() {
	gpu := driver.sprites
	if gpu == nil {
		return
	}

	for _, page := range gpu.pages {
		page.texture.Call("destroy")
	}

	if !gpu.buffer.IsUndefined() {
		gpu.buffer.Call("destroy")
	}

	if !gpu.uniform.IsUndefined() {
		gpu.uniform.Call("destroy")
	}

	driver.sprites = nil
}
