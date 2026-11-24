//go:build js && wasm

package browser

import (
	"fmt"
	"syscall/js"

	"github.com/karty-game/ashura/internal/render"
)

type gpuClusteredLights struct {
	state                        render.ClusterFrameState
	spots                        render.SpotShadowState
	uniform, uniformBytes, masks js.Value
	build, buildGroup            js.Value
	materials                    [4]js.Value
	spotPipeline                 js.Value
	spotTargets                  [render.SpotShadowCount]gpuSpotTarget
}

type gpuSpotTarget struct {
	image, view, uniform, bytes, group, descriptor js.Value
	matrix                                         [render.SpotMatrixBytes]byte
}

func RegisterClusteredLights(host any) error {
	driver, err := materialFeatureDriver(host)
	if err != nil {
		return err
	}

	lighting := &gpuClusteredLights{}

	driver.clusteredLights = lighting
	if err := lighting.prepareBuild(driver.device); err != nil {
		return err
	}

	return lighting.prepareSpots(driver.device)
}

func (lighting *gpuClusteredLights) prepareBuild(device js.Value) error {
	const storageUsage = 0x80

	lighting.uniform = device.Call("createBuffer", jsObject(map[string]any{
		labelProperty: "clustered light uniforms", sizeProperty: render.ClusterUniformSize, usageProperty: uniformBufferUsage,
	}))
	lighting.uniformBytes = js.Global().Get("Uint8Array").New(render.ClusterUniformSize)
	lighting.masks = device.Call("createBuffer", jsObject(map[string]any{
		labelProperty: "clustered light masks", sizeProperty: render.ClusterMaskBytes, usageProperty: storageUsage,
	}))
	module := device.Call("createShaderModule", jsObject(map[string]any{
		labelProperty: "clustered light culling", shaderCodeProperty: render.ClusterBuildShader(),
	}))

	pipeline, err := awaitPromise(device.Call("createComputePipelineAsync", jsObject(map[string]any{
		layoutProperty: automaticLayout, "compute": jsObject(map[string]any{
			moduleProperty: module, entryPointProperty: "buildLightMasks",
		}),
	})))
	if err != nil {
		return fmt.Errorf("create clustered light compute pipeline: %w", err)
	}

	lighting.build = pipeline
	lighting.buildGroup = device.Call("createBindGroup", jsObject(map[string]any{
		layoutProperty: pipeline.Call("getBindGroupLayout", 0), entriesProperty: js.ValueOf(lighting.bufferEntries()),
	}))

	return nil
}

func clusteredBufferEntry(binding int, buffer js.Value) js.Value {
	return jsObject(map[string]any{
		bindingProperty: binding, resourceProperty: jsObject(map[string]any{gpuBufferProperty: buffer}),
	})
}

func (lighting *gpuClusteredLights) bufferEntries() []any {
	return []any{clusteredBufferEntry(0, lighting.uniform), clusteredBufferEntry(1, lighting.masks)}
}

func (driver *Driver) prepareClusterMaterialGroups() {
	lighting := driver.clusteredLights
	if lighting == nil {
		return
	}

	const firstSpotBinding, secondSpotBinding = 2, 3

	for variant := range lighting.materials {
		pipeline := driver.materialPipeline(variant >= 2, variant%2 == 1)
		if pipeline.IsUndefined() || pipeline.IsNull() {
			continue
		}

		entries := append(lighting.bufferEntries(),
			jsObject(map[string]any{bindingProperty: firstSpotBinding, resourceProperty: lighting.spotTargets[0].view}),
			jsObject(map[string]any{bindingProperty: secondSpotBinding, resourceProperty: lighting.spotTargets[1].view}))
		lighting.materials[variant] = driver.device.Call("createBindGroup", jsObject(map[string]any{
			layoutProperty: pipeline.Call("getBindGroupLayout", 2), entriesProperty: js.ValueOf(entries),
		}))
	}
}

func (driver *Driver) recordClusteredLighting(encoder js.Value, frame render.Frame) {
	lighting := driver.clusteredLights
	if lighting == nil || len(frame.Draws) == 0 {
		return
	}

	dirty, masks := lighting.spots.Update(frame)
	//nolint:gosec // Visible canvas dimensions are positive and admitted by the WebGPU device.
	upload, dispatch := lighting.state.Update(frame, uint32(driver.canvasWidth), uint32(driver.canvasHeight), lighting.spots.Views)
	if upload {
		js.CopyBytesToJS(lighting.uniformBytes, lighting.state.Data[:])
		driver.queue.Call("writeBuffer", lighting.uniform, 0, lighting.uniformBytes)
	}

	if dispatch {
		encoder.Call("beginComputePass")

		pass := driver.computePass
		pass.Call("setPipeline", lighting.build)
		pass.Call("setBindGroup", 0, lighting.buildGroup)
		groups := lighting.state.Groups
		pass.Call("dispatchWorkgroups", groups[0], groups[1], groups[2])
		pass.Call("end")
		driver.metrics.Dispatch()
	}

	if dirty != 0 {
		driver.drawSpotShadows(encoder, dirty, masks)
	}
}

func (lighting *gpuClusteredLights) release() {
	for _, buffer := range []js.Value{lighting.uniform, lighting.masks} {
		if !buffer.IsUndefined() && !buffer.IsNull() {
			buffer.Call("destroy")
		}
	}

	for _, target := range lighting.spotTargets {
		for _, resource := range []js.Value{target.uniform, target.image} {
			if !resource.IsUndefined() && !resource.IsNull() {
				resource.Call("destroy")
			}
		}
	}
}
