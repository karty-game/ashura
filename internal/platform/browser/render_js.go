//go:build js && wasm

package browser

import (
	"fmt"
	"syscall/js"

	"github.com/karty-game/karty-webgpu/internal/diagnostics"
	"github.com/karty-game/karty-webgpu/internal/render"
	"github.com/karty-game/karty-webgpu/internal/uipaint"
)

func (driver *Driver) present(frame render.Frame) error {
	defer driver.pruneMaterials()
	defer driver.geometry.Prune(gpuGeometry.release)
	defer driver.pruneInstances()

	if err := driver.uploadComputeBuffers(frame.Uploads); err != nil {
		return err
	}

	if len(frame.Draws) > 0 {
		if err := driver.ensureTargets(); err != nil {
			return err
		}
	}

	if err := driver.prepareDrawData(frame); err != nil {
		return err
	}

	if err := driver.updateAttachedUI(frame.UI); err != nil {
		return err
	}

	if err := driver.updateSprites(frame.Screen2D); err != nil {
		return err
	}

	driver.metrics.Step(diagnostics.PreparePhase)

	texture := driver.context.Call("getCurrentTexture")
	if texture.IsUndefined() || texture.IsNull() {
		return errSurfaceTexture
	}

	view := texture.Call("createView")
	if view.IsUndefined() || view.IsNull() {
		return errSurfaceView
	}

	encoder := driver.device.Call("createCommandEncoder")
	if encoder.IsUndefined() || encoder.IsNull() {
		return errCommandEncoder
	}

	driver.metrics.Step(diagnostics.SurfacePhase)

	if err := driver.recordCompute(encoder, frame.Compute); err != nil {
		return err
	}

	driver.metrics.Step(diagnostics.ComputePhase)

	if err := driver.recordPointShadow(encoder, frame); err != nil {
		return err
	}

	if err := driver.drawScene(encoder, view, frame); err != nil {
		return err
	}

	driver.drawSprites(encoder, view)

	if driver.uiBridge != nil {
		driver.uiBridge.draw(encoder, view, frame.UI)
	}

	driver.metrics.Step(diagnostics.RenderPhase)

	return driver.submit(encoder)
}

func (driver *Driver) updateAttachedUI(frame *uipaint.Frame) error {
	if driver.uiBridge != nil {
		return driver.uiBridge.update(frame)
	}

	if frame != nil {
		return errDrawBuffer
	}

	return nil
}

func (driver *Driver) beginScenePass(encoder, view js.Value, clearColor [4]float32) (js.Value, error) {
	if clearColor == ([4]float32{}) {
		clearColor[3] = 1
	}

	driver.clearValue.Set("r", float64(clearColor[0]))
	driver.clearValue.Set("g", float64(clearColor[1]))
	driver.clearValue.Set("b", float64(clearColor[2]))
	driver.clearValue.Set("a", float64(clearColor[3]))
	driver.attachColor(view)
	driver.depthAttach.Set(viewProperty, driver.depthView)

	pass := encoder.Call("beginRenderPass", driver.passDescriptor)
	if pass.IsUndefined() || pass.IsNull() {
		return js.Undefined(), errRenderPass
	}

	return pass, nil
}

func (driver *Driver) submit(encoder js.Value) error {
	commands := encoder.Call("finish")
	if commands.IsUndefined() || commands.IsNull() {
		return errCommandBuffer
	}

	queue := driver.queue
	if driver.submitBuffers.IsUndefined() {
		driver.submitBuffers = js.Global().Get("Array").New(1)
	}

	driver.submitBuffers.SetIndex(0, commands)
	queue.Call("submit", driver.submitBuffers)
	driver.submitBuffers.SetIndex(0, js.Undefined())
	driver.metrics.Step(diagnostics.SubmitPhase)

	return nil
}

const textureUsageRenderAttachment = 0x10

func (driver *Driver) materialPipeline(source, cull bool) js.Value {
	if source {
		if cull {
			return driver.sourceCulledPipeline
		}

		return driver.sourcePipeline
	}

	if cull {
		return driver.standardCulledPipeline
	}

	return driver.standardPipeline
}

func (driver *Driver) createPipeline(format, shaderName, code string, source bool) error {
	shader := driver.device.Call(
		"createShaderModule",
		jsObject(map[string]any{labelProperty: shaderName, shaderCodeProperty: code}),
	)
	for _, cull := range []bool{false, true} {
		if err := driver.createPipelineVariant(format, shaderName, source, cull, false, shader); err != nil {
			return err
		}
	}

	return nil
}

func (driver *Driver) createPipelineVariant(format, shaderName string, source, cull, impostor bool, shader js.Value) error {
	cullMode := cullModeNone
	if cull {
		cullMode = "back"
	}

	buffers := modelVertexBuffers(source, impostor)

	targets := js.Global().Get("Array").New(1)
	targets.SetIndex(0, jsObject(map[string]any{formatProperty: format}))

	pipeline, err := awaitPromise(driver.device.Call("createRenderPipelineAsync", jsObject(map[string]any{
		labelProperty:  shaderName,
		layoutProperty: automaticLayout,
		vertexProperty: jsObject(map[string]any{moduleProperty: shader, entryPointProperty: vertexEntry, "buffers": buffers}),
		fragmentProperty: jsObject(
			map[string]any{moduleProperty: shader, entryPointProperty: render.FragmentEntry(true), targetsProperty: targets},
		),
		primitiveProperty: jsObject(map[string]any{topologyProperty: triangleListTopology, "frontFace": "ccw", cullModeProperty: cullMode}),
		depthStencilProperty: jsObject(
			map[string]any{formatProperty: "depth24plus", depthWriteEnabledProperty: true, depthCompareProperty: "less"},
		),
		"multisample": jsObject(map[string]any{"count": driver.sampleCount}),
	})))
	if err != nil {
		return fmt.Errorf("create %s pipeline: %w", shaderName, err)
	}

	if pipeline.IsUndefined() || pipeline.IsNull() {
		return errRenderPipeline
	}

	switch {
	case impostor:
		driver.impostorPipeline = pipeline
	case source && cull:
		driver.sourceCulledPipeline = pipeline
	case source:
		driver.sourcePipeline = pipeline
	case cull:
		driver.standardCulledPipeline = pipeline
	default:
		driver.standardPipeline = pipeline
	}

	return nil
}

func (driver *Driver) prepareScenePass() error {
	driver.indexFormat = js.ValueOf("uint32")
	driver.storeOperation = js.ValueOf("store")
	driver.discardOperation = js.ValueOf("discard")

	driver.clearValue = jsObject(map[string]any{"r": 0, "g": 0, "b": 0, "a": 1})
	driver.colorAttach = jsObject(map[string]any{
		viewProperty: js.Undefined(), "clearValue": driver.clearValue, loadOpProperty: loadOpClear, storeOpProperty: uiStore,
	})
	driver.depthAttach = jsObject(map[string]any{
		viewProperty: js.Undefined(), "depthLoadOp": loadOpClear, "depthStoreOp": "discard", "depthClearValue": 1,
	})
	driver.attachments = js.Global().Get("Array").New(1)
	driver.attachments.SetIndex(0, driver.colorAttach)
	driver.passDescriptor = jsObject(map[string]any{
		uiAttachments: driver.attachments, "depthStencilAttachment": driver.depthAttach,
	})

	return nil
}

func instanceVertexAttributes() js.Value {
	const (
		modelColumns  = 4
		vectorBytes   = 16
		modelLocation = 3
	)

	attributes := js.Global().Get("Array").New(modelColumns)
	for index := range modelColumns {
		attributes.SetIndex(index, jsObject(map[string]any{
			formatProperty: "float32x4", offsetProperty: index * vectorBytes, shaderLocationProperty: index + modelLocation,
		}))
	}

	return attributes
}

func (driver *Driver) prepareDrawData(frame render.Frame) error {
	if err := driver.geometry.Prepare(frame.Draws); err != nil {
		return err
	}

	clear(driver.draws)
	driver.draws = driver.draws[:0]

	for _, draw := range frame.Draws {
		if !render.HasGeometry(draw) {
			continue
		}

		geometry, err := driver.geometry.Resolve(draw, driver.createGeometry)
		if err != nil {
			return err
		}

		instances, err := driver.instanceBuffer(draw)
		if err != nil {
			return err
		}

		driver.draws = append(
			driver.draws,
			browserDraw{
				call:      draw,
				geometry:  geometry,
				count:     render.IndexCount(draw),
				instance:  instances,
				instances: instances.state.Count(),
			},
		)
	}

	return nil
}

func nextCapacity(required int) int {
	capacity := 256
	for capacity < required {
		capacity *= 2
	}

	return capacity
}

func (driver *Driver) drawMaterials(pass js.Value, frame render.Frame) error {
	var activeSource, activeCull, activeImpostor bool

	for index, draw := range driver.draws {
		group, err := driver.materialGroup(draw.call, frame)
		if err != nil {
			return err
		}

		if index == 0 || activeSource != draw.call.Source || activeCull != draw.call.CullBackfaces ||
			activeImpostor != (draw.call.Impostor != nil) {
			pass.Call("setPipeline", driver.drawPipeline(draw.call))
			// The shadow group is shared by this pipeline variant. Rebinding it
			// per draw needlessly allocates JS bridge call arguments.
			driver.bindPointShadow(pass, draw.call)
			activeSource, activeCull = draw.call.Source, draw.call.CullBackfaces
			activeImpostor = draw.call.Impostor != nil
		}

		pass.Call("setVertexBuffer", 0, draw.geometry.vertices)
		pass.Call("setVertexBuffer", 1, draw.instance.buffer)

		if draw.call.Impostor == nil {
			uv := draw.geometry.lightmapUV
			if uv.IsUndefined() {
				uv = draw.geometry.vertices
			}

			pass.Call("setVertexBuffer", 2, uv)
		}

		pass.Call("setIndexBuffer", draw.geometry.indices, driver.indexFormat)
		pass.Call("setBindGroup", 0, group)
		// WebGPU defaults firstIndex, baseVertex and firstInstance to zero.
		pass.Call("drawIndexed", draw.count, draw.instances)
		driver.metrics.Draw(
			uint64(draw.count),     //nolint:gosec // Prepared index counts come from nonnegative slice lengths.
			uint64(draw.instances), //nolint:gosec // Prepared instance counts come from nonnegative slice lengths.
		)
	}

	return nil
}

func modelVertexBuffers(_, impostor bool) js.Value {
	attributes := js.Global().Get("Array").New(3)
	attributes.SetIndex(0, jsObject(map[string]any{formatProperty: formatFloat32x3, offsetProperty: 0, shaderLocationProperty: 0}))
	attributes.SetIndex(
		1,
		jsObject(map[string]any{formatProperty: formatFloat32x3, offsetProperty: render.NormalOffset, shaderLocationProperty: 1}),
	)
	attributes.SetIndex(
		2,
		jsObject(map[string]any{formatProperty: "float32x2", offsetProperty: render.UVOffset, shaderLocationProperty: render.UVLocation}),
	)

	instanceAttributes := instanceVertexAttributes()

	buffers := js.Global().Get("Array").New(2)
	buffers.SetIndex(
		0,
		jsObject(
			map[string]any{arrayStrideProperty: render.VertexStride, stepModeProperty: vertexProperty, attributesProperty: attributes},
		),
	)
	buffers.SetIndex(
		1,
		jsObject(
			map[string]any{
				arrayStrideProperty: render.InstanceStride,
				stepModeProperty:    "instance",
				attributesProperty:  instanceAttributes,
			},
		),
	)

	if !impostor {
		buffers.SetIndex(
			2,
			jsObject(
				map[string]any{
					arrayStrideProperty: render.LightmapUVStride,
					stepModeProperty:    vertexProperty,
					attributesProperty: js.ValueOf(
						[]any{
							jsObject(
								map[string]any{
									formatProperty:         "float32x2",
									offsetProperty:         0,
									shaderLocationProperty: render.LightmapUVLocation,
								},
							),
						},
					),
				},
			),
		)
	}

	return buffers
}
