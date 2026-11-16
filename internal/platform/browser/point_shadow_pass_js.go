//go:build js && wasm

package browser

import (
	"fmt"
	"syscall/js"

	"github.com/karty-game/karty-webgpu/internal/render"
)

func (driver *pointShadowFeature) initPointShadowPasses(shadow *pointShadowGPU) error {
	if !shadow.pipelines[0].IsUndefined() && !shadow.pipelines[0].IsNull() {
		return nil
	}

	for face := range render.PointShadowFaces {
		shadow.faces[face] = shadow.image.Call(
			"createView",
			jsObject(map[string]any{dimensionProperty: "2d", "baseArrayLayer": face, arrayLayerCountProperty: 1}),
		)
		shadow.uniforms[face] = driver.device.Call(
			"createBuffer",
			jsObject(map[string]any{sizeProperty: render.PointShadowFaceSize, usageProperty: uniformBufferUsage}),
		)
	}

	shader := driver.device.Call(
		"createShaderModule",
		jsObject(map[string]any{labelProperty: "point-shadow.wgsl", shaderCodeProperty: render.PointShadowShader()}),
	)
	for variant := range 2 {
		if err := driver.createPointShadowVariant(shadow, shader, variant); err != nil {
			return err
		}
	}

	return nil
}

func (driver *pointShadowFeature) createPointShadowVariant(shadow *pointShadowGPU, shader js.Value, variant int) error {
	cull := cullModeNone
	if variant == 1 {
		cull = "back"
	}

	positionAttributes := []any{jsObject(map[string]any{formatProperty: formatFloat32x3, offsetProperty: 0, shaderLocationProperty: 0})}
	buffers := []any{
		jsObject(
			map[string]any{
				arrayStrideProperty: render.VertexStride,
				stepModeProperty:    vertexProperty,
				attributesProperty:  js.ValueOf(positionAttributes),
			},
		),
		jsObject(
			map[string]any{
				arrayStrideProperty: render.InstanceStride,
				stepModeProperty:    "instance",
				attributesProperty:  instanceVertexAttributes(),
			},
		),
	}

	pipeline, err := awaitPromise(
		driver.device.Call(
			"createRenderPipelineAsync",
			jsObject(
				map[string]any{
					labelProperty:  "point shadow radial depth",
					layoutProperty: automaticLayout,
					vertexProperty: jsObject(
						map[string]any{moduleProperty: shader, entryPointProperty: vertexEntry, "buffers": js.ValueOf(buffers)},
					),
					fragmentProperty: jsObject(
						map[string]any{moduleProperty: shader, entryPointProperty: "fs_main", targetsProperty: js.ValueOf([]any{})},
					),
					primitiveProperty: jsObject(
						map[string]any{topologyProperty: triangleListTopology, "frontFace": "cw", cullModeProperty: cull},
					),
					depthStencilProperty: jsObject(
						map[string]any{formatProperty: "depth32float", depthWriteEnabledProperty: true, depthCompareProperty: "less"},
					),
				},
			),
		),
	)
	if err != nil {
		return fmt.Errorf("create point shadow pipeline: %w", err)
	}

	shadow.pipelines[variant] = pipeline
	for face := range render.PointShadowFaces {
		entries := []any{
			jsObject(
				map[string]any{bindingProperty: 0, resourceProperty: jsObject(map[string]any{gpuBufferProperty: shadow.uniforms[face]})},
			),
		}
		shadow.groups[variant][face] = driver.device.Call(
			"createBindGroup",
			jsObject(map[string]any{layoutProperty: pipeline.Call("getBindGroupLayout", 0), entriesProperty: js.ValueOf(entries)}),
		)
	}

	return nil
}

func (driver *pointShadowFeature) recordPointShadow(encoder js.Value, frame render.Frame) error {
	light, enabled := render.ShadowLight(frame)

	shadow := driver.pointShadow
	if shadow == nil ||
		enabled && (shadow.pipelines[0].IsUndefined() || shadow.pipelines[0].IsNull() || shadow.resolution != light.ShadowResolution) {
		return render.ErrPointShadowStartup
	}

	if !enabled {
		light.GroundColor[0] = 1
	}

	data := render.PointShadowSampleUniform(light, enabled)
	if data != shadow.sampleData {
		array := js.Global().Get("Uint8Array").New(len(data))
		js.CopyBytesToJS(array, data[:])
		driver.queue.Call("writeBuffer", shadow.sampleUniform, 0, array)
		shadow.sampleData = data
	}

	if !enabled || !shadow.state.Update(frame, light) {
		return nil
	}

	for face := range render.PointShadowFaces {
		data := render.PointShadowFaceUniform(light, face)
		array := js.Global().Get("Uint8Array").New(len(data))
		js.CopyBytesToJS(array, data[:])
		driver.queue.Call("writeBuffer", shadow.uniforms[face], 0, array)
		pass := driver.startRenderPass(
			encoder,
			jsObject(
				map[string]any{
					labelProperty: "point shadow face",
					uiAttachments: js.ValueOf([]any{}),
					"depthStencilAttachment": jsObject(
						map[string]any{
							viewProperty:      shadow.faces[face],
							"depthLoadOp":     loadOpClear,
							"depthStoreOp":    uiStore,
							"depthClearValue": 1,
						},
					),
				},
			),
		)

		for _, draw := range driver.draws {
			if !render.CastsPointShadow(draw.call) {
				continue
			}

			variant := 0
			if draw.call.CullBackfaces {
				variant = 1
			}

			pass.Call("setPipeline", shadow.pipelines[variant])
			pass.Call("setBindGroup", 0, shadow.groups[variant][face])
			pass.Call("setVertexBuffer", 0, draw.geometry.vertices)
			pass.Call("setVertexBuffer", 1, draw.instance.buffer)
			pass.Call("setIndexBuffer", draw.geometry.indices, driver.indexFormat)
			// WebGPU defaults all trailing offsets to zero.
			pass.Call("drawIndexed", draw.count, draw.instances)
		}

		pass.Call("end")
	}

	return nil
}
