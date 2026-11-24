//go:build js && wasm

package browser

import (
	"fmt"
	"syscall/js"

	"github.com/karty-game/ashura/internal/render"
)

const spotDepthLabel = "spot shadow depth"

func (lighting *gpuClusteredLights) prepareSpots(device js.Value) error {
	module := device.Call("createShaderModule", jsObject(map[string]any{
		labelProperty: spotDepthLabel, shaderCodeProperty: render.ShadowDepthShader(),
	}))
	positionAttributes := []any{jsObject(map[string]any{formatProperty: formatFloat32x3, offsetProperty: 0, shaderLocationProperty: 0})}
	buffers := []any{
		jsObject(map[string]any{
			arrayStrideProperty: render.VertexStride, stepModeProperty: vertexProperty, attributesProperty: js.ValueOf(positionAttributes),
		}),
		jsObject(map[string]any{
			arrayStrideProperty: render.InstanceStride, stepModeProperty: uiInstanceStep, attributesProperty: instanceVertexAttributes(),
		}),
	}

	pipeline, err := awaitPromise(device.Call("createRenderPipelineAsync", jsObject(map[string]any{
		labelProperty:  spotDepthLabel,
		layoutProperty: automaticLayout,
		vertexProperty: jsObject(
			map[string]any{moduleProperty: module, entryPointProperty: vertexEntry, uiVertexBuffers: js.ValueOf(buffers)},
		),
		primitiveProperty: jsObject(map[string]any{topologyProperty: triangleListTopology, cullModeProperty: cullModeNone}),
		depthStencilProperty: jsObject(map[string]any{
			formatProperty: shadowDepthFormat, depthWriteEnabledProperty: true, depthCompareProperty: depthCompareLess,
			"depthBias": render.ShadowDepthBias, "depthBiasSlopeScale": render.ShadowDepthBiasSlopeScale,
		}),
	})))
	if err != nil {
		return fmt.Errorf("create spotlight shadow pipeline: %w", err)
	}

	lighting.spotPipeline = pipeline

	layout := pipeline.Call("getBindGroupLayout", 0)
	for slot := range lighting.spotTargets {
		lighting.spotTargets[slot].init(device, layout, render.SpotShadowResolution)
	}

	return nil
}

func (target *gpuSpotTarget) init(device, layout js.Value, resolution uint32) {
	target.image = device.Call("createTexture", jsObject(map[string]any{
		labelProperty: spotDepthLabel, formatProperty: shadowDepthFormat, usageProperty: pointShadowTextureUsage,
		sizeProperty: jsObject(map[string]any{widthProperty: resolution, heightProperty: resolution, depthOrArrayLayersProperty: 1}),
	}))
	target.view = target.image.Call("createView")
	target.uniform = device.Call("createBuffer", jsObject(map[string]any{
		labelProperty: "spot shadow matrix", sizeProperty: render.SpotMatrixBytes, usageProperty: uniformBufferUsage,
	}))
	target.bytes = js.Global().Get("Uint8Array").New(render.SpotMatrixBytes)
	target.group = device.Call("createBindGroup", jsObject(map[string]any{
		layoutProperty: layout, entriesProperty: js.ValueOf([]any{clusteredBufferEntry(0, target.uniform)}),
	}))
	target.descriptor = jsObject(map[string]any{
		labelProperty: spotDepthLabel, uiAttachments: js.ValueOf([]any{}),
		drawDepthAttachment: jsObject(map[string]any{
			viewProperty: target.view, depthLoadProperty: loadOpClear, depthStoreProperty: uiStore, depthClearProperty: 1,
		}),
	})
}

func (driver *Driver) drawSpotShadows(encoder js.Value, dirty uint8, masks []uint8) {
	lighting := driver.clusteredLights
	for slot := range lighting.spotTargets {
		bit := uint8(1 << slot)
		if dirty&bit == 0 {
			continue
		}

		target := &lighting.spotTargets[slot]

		matrix := lighting.spots.Views[slot].Uniform()
		if matrix != target.matrix {
			js.CopyBytesToJS(target.bytes, matrix[:])
			driver.queue.Call("writeBuffer", target.uniform, 0, target.bytes)
			target.matrix = matrix
		}

		pass := driver.startRenderPass(encoder, target.descriptor)
		pass.Call("setPipeline", lighting.spotPipeline)
		pass.Call("setBindGroup", 0, target.group)

		for index, draw := range driver.draws {
			if masks[index]&bit == 0 {
				continue
			}

			pass.Call("setVertexBuffer", 0, draw.geometry.vertices)
			pass.Call("setVertexBuffer", 1, draw.instance.buffer)
			pass.Call("setIndexBuffer", draw.geometry.indices, driver.indexFormat)
			pass.Call("drawIndexed", draw.count, draw.instances)
		}

		pass.Call("end")
	}
}
