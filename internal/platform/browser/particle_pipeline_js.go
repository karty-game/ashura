//go:build js && wasm

package browser

import (
	"syscall/js"

	"github.com/karty-game/ashura/internal/particledata"
)

const (
	particleSampleCountProperty = "count"
	particleDepthFormat         = "depth24plus"
	particleMinBindingProperty  = "minBindingSize"
)

func (feature *particleFeature) prepareParticleDraw(module js.Value, spaces [particledata.Spaces]bool) error {
	device := feature.device

	feature.drawLayout = device.Call("createBindGroupLayout", jsObject(map[string]any{
		entriesProperty: particleBufferLayout(1, false),
	}))
	texture := device.Call("createBindGroupLayout", jsObject(map[string]any{entriesProperty: []any{
		jsObject(
			map[string]any{
				bindingProperty:            0,
				particleVisibilityProperty: particleFragmentStage,
				particleTextureProperty:    jsObject(map[string]any{"sampleType": "float"}),
			},
		),
		jsObject(
			map[string]any{
				bindingProperty:            1,
				particleVisibilityProperty: particleFragmentStage,
				"sampler":                  jsObject(map[string]any{bindingTypeProperty: "filtering"}),
			},
		),
	}}))

	for space, needed := range spaces {
		if needed {
			if err := feature.prepareSpace(module, texture, particledata.Space(space)); err != nil {
				return err
			}
		}
	}

	feature.sampler = device.Call(
		"createSampler",
		jsObject(map[string]any{magFilterProperty: linearFilter, minFilterProperty: linearFilter}),
	)

	return nil
}

func (feature *particleFeature) prepareSpace(module, texture js.Value, space particledata.Space) error {
	device := feature.device
	layouts := []any{feature.drawLayout, texture}
	entry := "particleVertex"

	if space == particledata.WorldSpace {
		layouts = append(layouts, feature.prepareParticleCamera())
		entry = "worldParticleVertex"
	}

	drawLayout := device.Call("createPipelineLayout", jsObject(map[string]any{
		particleLayoutsProperty: layouts,
	}))

	for index := range feature.pipelines[space] {
		dst := particleOver
		if particledata.Blend(index) == particledata.Add {
			dst = particleOne
		}

		color := jsObject(
			map[string]any{particleSourceFactor: particleOne, particleDestinationFactor: dst, particleBlendOperation: particleAdd},
		)
		alpha := jsObject(
			map[string]any{particleSourceFactor: particleOne, particleDestinationFactor: particleOver, particleBlendOperation: particleAdd},
		)

		descriptor := jsObject(map[string]any{
			labelProperty:  entry,
			layoutProperty: drawLayout,
			vertexProperty: jsObject(
				map[string]any{moduleProperty: module, entryPointProperty: entry, uiVertexBuffers: particleVertexLayout()},
			),
			fragmentProperty: jsObject(map[string]any{
				moduleProperty: module, entryPointProperty: "particleFragment",
				targetsProperty: []any{jsObject(map[string]any{
					formatProperty:        feature.colorFormat,
					particleBlendProperty: jsObject(map[string]any{particleColorProperty: color, particleAlphaProperty: alpha}),
				})},
			}),
		})
		if space == particledata.WorldSpace {
			descriptor.Set("multisample", jsObject(map[string]any{particleSampleCountProperty: feature.sampleCount}))
			descriptor.Set(depthStencilProperty, jsObject(map[string]any{
				formatProperty: particleDepthFormat, depthWriteEnabledProperty: false, depthCompareProperty: depthCompareLess,
			}))
		}

		pipeline, err := awaitPromise(device.Call("createRenderPipelineAsync", descriptor))
		if err != nil {
			return err
		}

		feature.pipelines[space][index] = pipeline
	}

	return nil
}

func (feature *particleFeature) prepareParticleCamera() js.Value {
	const vertexStage = 1

	layout := feature.device.Call("createBindGroupLayout", jsObject(map[string]any{
		entriesProperty: []any{jsObject(map[string]any{
			bindingProperty:            0,
			particleVisibilityProperty: vertexStage,
			gpuBufferProperty: jsObject(
				map[string]any{bindingTypeProperty: "uniform", particleMinBindingProperty: particledata.CameraBytes},
			),
		})},
	}))
	feature.cameraBuffer = feature.particleBuffer("Particle camera", particledata.CameraBytes, 0x48)
	feature.cameraUpload = js.Global().Get("Uint8Array").New(particledata.CameraBytes)
	feature.cameraGroup = feature.device.Call("createBindGroup", jsObject(map[string]any{
		layoutProperty: layout,
		entriesProperty: []any{jsObject(map[string]any{
			bindingProperty: 0, resourceProperty: jsObject(map[string]any{gpuBufferProperty: feature.cameraBuffer}),
		})},
	}))

	return layout
}
