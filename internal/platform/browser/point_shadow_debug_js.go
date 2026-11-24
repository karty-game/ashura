//go:build js && wasm

package browser

import (
	"syscall/js"

	"github.com/karty-game/ashura/internal/render"
)

func (driver *pointShadowFeature) initPointShadowDebug() error {
	shadow := driver.pointShadow
	if !shadow.debugPipeline.IsUndefined() && !shadow.debugPipeline.IsNull() {
		return nil
	}

	shader := driver.device.Call(
		"createShaderModule",
		jsObject(map[string]any{labelProperty: "point-shadow-debug.wgsl", shaderCodeProperty: render.PointShadowDebugShader()}),
	)

	pipeline, err := awaitPromise(
		driver.device.Call(
			"createRenderPipelineAsync",
			jsObject(
				map[string]any{
					labelProperty:  "point shadow debug faces",
					layoutProperty: automaticLayout,
					vertexProperty: jsObject(map[string]any{moduleProperty: shader, entryPointProperty: vertexEntry}),
					fragmentProperty: jsObject(
						map[string]any{
							moduleProperty:     shader,
							entryPointProperty: "fs_main_srgb",
							targetsProperty:    js.ValueOf([]any{jsObject(map[string]any{formatProperty: driver.colorFormat})}),
						},
					),
					primitiveProperty: jsObject(map[string]any{topologyProperty: triangleListTopology, cullModeProperty: cullModeNone}),
					depthStencilProperty: jsObject(
						map[string]any{formatProperty: "depth24plus", depthWriteEnabledProperty: false, depthCompareProperty: "always"},
					),
					"multisample": jsObject(map[string]any{"count": driver.sampleCount}),
				},
			),
		),
	)
	if err != nil {
		return err
	}

	shadow.debugPipeline = pipeline
	entries := []any{
		jsObject(map[string]any{bindingProperty: 0, resourceProperty: shadow.layers}),
		jsObject(map[string]any{bindingProperty: 1, resourceProperty: shadow.sampler}),
	}
	shadow.debugGroup = driver.device.Call(
		"createBindGroup",
		jsObject(map[string]any{layoutProperty: pipeline.Call("getBindGroupLayout", 0), entriesProperty: js.ValueOf(entries)}),
	)

	return nil
}

func (driver *pointShadowFeature) drawPointShadowDebug(pass js.Value, frame render.Frame) {
	light, enabled := render.ShadowLight(frame)
	if !enabled || !light.DebugShadowFaces {
		return
	}

	pass.Call("setPipeline", driver.pointShadow.debugPipeline)
	pass.Call("setBindGroup", 0, driver.pointShadow.debugGroup)
	pass.Call("draw", 3, 1, 0, 0)
}
