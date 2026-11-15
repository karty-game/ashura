//go:build js && wasm

package browser

import (
	"syscall/js"

	"github.com/karty-game/karty-webgpu/internal/render"
)

// gpuPointShadow stores only the material sampling ABI shared by optional renderers.
type gpuPointShadow struct {
	resolution                          uint32
	image, cube, sampleUniform, sampler js.Value
	sampleGroups                        [4]js.Value
	sampleData                          [render.PointShadowSampleSize]byte
}

// preparePointShadowSampling retains the common material binding ABI only.
// Optional shadow passes and their WGSL are installed by RegisterPointShadows.
func (driver *Driver) preparePointShadowSampling(resolution uint32) error {
	if (driver.standardPipeline.IsUndefined() || driver.standardPipeline.IsNull()) &&
		(driver.sourcePipeline.IsUndefined() || driver.sourcePipeline.IsNull()) {
		return nil
	}

	if resolution == 0 {
		resolution = 1
	}

	return driver.initPointShadow(resolution)
}

func (driver *Driver) initPointShadow(resolution uint32) error {
	if driver.pointShadow != nil && driver.pointShadow.resolution == resolution {
		return nil
	}

	if driver.pointShadow != nil {
		driver.pointShadow.release()
	}

	shadow := &gpuPointShadow{resolution: resolution}
	driver.pointShadow = shadow
	shadow.image = driver.device.Call(
		"createTexture",
		jsObject(
			map[string]any{
				labelProperty:                 "point shadow six faces",
				"textureBindingViewDimension": "cube",
				formatProperty:                "depth32float",
				usageProperty:                 pointShadowTextureUsage,
				sizeProperty: jsObject(
					map[string]any{
						widthProperty:              resolution,
						heightProperty:             resolution,
						depthOrArrayLayersProperty: render.PointShadowFaces,
					},
				),
			},
		),
	)
	shadow.cube = shadow.image.Call(
		"createView",
		jsObject(map[string]any{dimensionProperty: "cube", "baseArrayLayer": 0, arrayLayerCountProperty: render.PointShadowFaces}),
	)
	shadow.sampler = driver.device.Call(
		"createSampler",
		jsObject(
			map[string]any{
				"compare":            "less-equal",
				addressModeUProperty: clampToEdge,
				addressModeVProperty: clampToEdge,
				"addressModeW":       clampToEdge,
				minFilterProperty:    linearFilter,
				magFilterProperty:    linearFilter,
			},
		),
	)
	shadow.sampleUniform = driver.device.Call(
		"createBuffer",
		jsObject(map[string]any{sizeProperty: render.PointShadowSampleSize, usageProperty: uniformBufferUsage}),
	)
	driver.initPointShadowSampleGroups(shadow)

	return nil
}

func (driver *Driver) initPointShadowSampleGroups(shadow *gpuPointShadow) {
	for variant := range 4 {
		pipeline := driver.materialPipeline(variant >= 2, variant%2 == 1)
		if pipeline.IsUndefined() || pipeline.IsNull() {
			continue
		}

		entries := []any{
			jsObject(
				map[string]any{bindingProperty: 0, resourceProperty: jsObject(map[string]any{gpuBufferProperty: shadow.sampleUniform})},
			),
			jsObject(map[string]any{bindingProperty: 1, resourceProperty: shadow.sampler}),
			jsObject(map[string]any{bindingProperty: pointShadowTextureBinding, resourceProperty: shadow.cube}),
		}
		shadow.sampleGroups[variant] = driver.device.Call(
			"createBindGroup",
			jsObject(
				map[string]any{
					layoutProperty:  pipeline.Call("getBindGroupLayout", 1),
					entriesProperty: js.ValueOf(entries),
				},
			),
		)
	}
}

func (driver *Driver) bindPointShadow(pass js.Value, draw render.DrawCall) {
	if draw.Impostor != nil {
		return
	}

	variant := 0
	if draw.CullBackfaces {
		variant = 1
	}

	if draw.Source {
		variant += 2
	}

	pass.Call("setBindGroup", 1, driver.pointShadow.sampleGroups[variant])
}

func (shadow *gpuPointShadow) release() {
	if !shadow.image.IsUndefined() && !shadow.image.IsNull() {
		shadow.image.Call("destroy")
	}

	if !shadow.sampleUniform.IsUndefined() && !shadow.sampleUniform.IsNull() {
		shadow.sampleUniform.Call("destroy")
	}
}
