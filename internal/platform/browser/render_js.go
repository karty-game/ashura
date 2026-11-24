//go:build js && wasm

package browser

import (
	"fmt"
	"syscall/js"

	"github.com/karty-game/ashura/internal/diagnostics"
	"github.com/karty-game/ashura/internal/render"
	"github.com/karty-game/ashura/internal/uipaint"
)

func (driver *Driver) present(frame render.Frame) error {
	defer driver.pruneMaterials()
	defer driver.geometry.Prune(gpuGeometry.release)
	defer driver.pruneInstances()

	if err := driver.uploadComputeBuffers(frame.Uploads); err != nil {
		return err
	}

	if len(frame.Draws) > 0 || frame.WorldParticles {
		if err := driver.ensureTargets(); err != nil {
			return err
		}
	}

	if err := driver.prepareDrawData(frame); err != nil {
		return err
	}

	if len(driver.draws) != 0 {
		if err := driver.updateFrameUniform(frame); err != nil {
			return err
		}
	}

	if err := driver.updateAttachedUI(frame.UI); err != nil {
		return err
	}

	if err := driver.updateSprites(frame.Screen2D); err != nil {
		return err
	}

	driver.metrics.Step(diagnostics.PreparePhase)

	if !driver.frameHost.Call("begin", driver.context, driver.device).Bool() {
		return errFrameStart
	}

	driver.frameOpen = true
	defer driver.abortFrame()

	view, encoder := driver.surfaceView, driver.frameEncoder

	driver.metrics.Step(diagnostics.SurfacePhase)

	if err := driver.recordCompute(encoder, frame.Compute); err != nil {
		return err
	}

	driver.simulateParticles(encoder, frame.Particles)

	driver.metrics.Step(diagnostics.ComputePhase)

	if err := driver.recordLighting(encoder, frame); err != nil {
		return err
	}

	if err := driver.drawScene(encoder, view, frame); err != nil {
		return err
	}

	driver.drawOverlays(encoder, view, frame.UI)

	driver.metrics.Step(diagnostics.RenderPhase)

	return driver.submit()
}

func (driver *Driver) recordLighting(encoder js.Value, frame render.Frame) error {
	if err := driver.recordPointShadow(encoder, frame); err != nil {
		return err
	}

	driver.recordClusteredLighting(encoder, frame)

	return driver.recordStaticDecals(encoder, frame)
}

func (driver *Driver) drawOverlays(encoder, view js.Value, overlay *uipaint.Frame) {
	driver.drawSprites(encoder, view)

	if driver.particleBridge != nil {
		driver.particleBridge.Draw(encoder, view)
	}

	if driver.uiBridge != nil {
		driver.uiBridge.draw(encoder, view, overlay)
	}
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

	pass := driver.startRenderPass(encoder, driver.passDescriptor)
	if pass.IsUndefined() || pass.IsNull() {
		return js.Undefined(), errRenderPass
	}

	return pass, nil
}

func (driver *Driver) submit() error {
	if !driver.frameHost.Call("submit", driver.queue).Bool() {
		return errCommandBuffer
	}

	driver.frameOpen = false
	driver.metrics.Step(diagnostics.SubmitPhase)

	return nil
}

const (
	shadowDepthFormat   = "depth32float"
	drawDepthAttachment = "depthStencilAttachment"
	depthLoadProperty   = "depthLoadOp"
	depthStoreProperty  = "depthStoreOp"
	depthClearProperty  = "depthClearValue"
	depthCompareLess    = "less"
)

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
		vertexProperty: jsObject(map[string]any{moduleProperty: shader, entryPointProperty: vertexEntry, uiVertexBuffers: buffers}),
		fragmentProperty: jsObject(
			map[string]any{moduleProperty: shader, entryPointProperty: render.FragmentEntry(true), targetsProperty: targets},
		),
		primitiveProperty: jsObject(map[string]any{topologyProperty: triangleListTopology, "frontFace": "ccw", cullModeProperty: cullMode}),
		depthStencilProperty: jsObject(
			map[string]any{formatProperty: "depth24plus", depthWriteEnabledProperty: true, depthCompareProperty: depthCompareLess},
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
		viewProperty: js.Undefined(), depthLoadProperty: loadOpClear, depthStoreProperty: "discard", depthClearProperty: 1,
	})
	driver.attachments = js.Global().Get("Array").New(1)
	driver.attachments.SetIndex(0, driver.colorAttach)
	driver.passDescriptor = jsObject(map[string]any{
		uiAttachments: driver.attachments, drawDepthAttachment: driver.depthAttach,
	})
	driver.clearPassDescriptor = jsObject(map[string]any{uiAttachments: driver.attachments})

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
			driver.draws = append(driver.draws, browserDraw{})

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
	if err := driver.prepareDecals(frame.Decals); err != nil {
		return err
	}

	var activeSource, activeCull, activeImpostor, pipelineSet bool

	order := driver.opaqueOrder.Prepare(frame)
	for index := range driver.draws {
		if order != nil {
			index = order[index]
		}

		draw := driver.draws[index]
		if draw.count == 0 {
			continue
		}

		group, err := driver.materialGroup(draw.call)
		if err != nil {
			return err
		}

		if !pipelineSet || activeSource != draw.call.Source || activeCull != draw.call.CullBackfaces ||
			activeImpostor != (draw.call.Impostor != nil) {
			pass.Call("setPipeline", driver.drawPipeline(draw.call))
			// The shadow group is shared by this pipeline variant. Rebinding it
			// per draw needlessly allocates JS bridge call arguments.
			driver.bindPointShadow(pass, draw.call)
			driver.bindDecals(pass, draw.call)
			activeSource, activeCull = draw.call.Source, draw.call.CullBackfaces
			activeImpostor = draw.call.Impostor != nil
			pipelineSet = true
		}

		lightmapUV := js.Undefined()
		if draw.call.Impostor == nil {
			lightmapUV = draw.geometry.lightmapUV
			if lightmapUV.IsUndefined() {
				lightmapUV = draw.geometry.vertices
			}
		}

		// Forward the existing bindings/draw together so TinyGo creates argument
		// storage once per draw rather than once per individual GPU command.
		pass.Call("drawMesh", draw.geometry.vertices, draw.instance.buffer, lightmapUV,
			draw.geometry.indices, driver.indexFormat, group, draw.count, draw.instances)
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
				stepModeProperty:    uiInstanceStep,
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
