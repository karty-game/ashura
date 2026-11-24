//go:build js && wasm

package browser

import (
	"errors"
	"fmt"
	"syscall/js"

	"github.com/karty-game/ashura/internal/particledata"
	"github.com/karty-game/ashura/internal/platform"
	"github.com/karty-game/ashura/internal/uipaint"
)

const (
	particleBlendOperation     = "operation"
	particleSourceFactor       = "srcFactor"
	particleColorProperty      = "color"
	particleVisibilityProperty = "visibility"
	particleDestinationFactor  = "dstFactor"
	particleBlendProperty      = "blend"
	particleFragmentStage      = 2
	particleLoad               = "load"
	particleLayoutsProperty    = "bindGroupLayouts"
	particleTextureProperty    = "texture"
	bindingTypeProperty        = "type"
	particleOver               = "one-minus-src-alpha"
	particleOne                = "one"
	particleAdd                = "add"
	particleAlphaProperty      = "alpha"
	particleComputeProperty    = "compute"
)

type particleBackend interface {
	Prepare(instances []*particledata.Instance) error
	Simulate(encoder js.Value, instances []*particledata.Instance)
	Draw(encoder, view js.Value)
	DrawWorld(pass js.Value, projection [16]float32)
	Close()
}

type particleFeature struct {
	*Driver

	advance, spawn                             js.Value
	pipelines                                  [particledata.Spaces][2]js.Value
	camera                                     particledata.Camera
	cameraBuffer, cameraGroup, cameraUpload    js.Value
	computeLayout, drawLayout, sampler, upload js.Value
	pages                                      map[*uipaint.Texture]uiPage
	configs                                    map[*particledata.Layer]js.Value
	warmBuffers                                map[*particledata.Layer]js.Value
	warmCopies                                 []particleWarmCopy
	layers                                     []particleLayer
	passAttachment, passDescriptor             js.Value
	screenActive                               bool
}

type particleWarmCopy struct {
	source, destination js.Value
	step                *particledata.Step
	size                int
}

type particleLayer struct {
	state, frame, computeGroup, drawGroup, page js.Value
	slots, capacity                             uint32
	blend                                       particledata.Blend
	active                                      bool
	space                                       particledata.Space
}

func RegisterParticles(host any) error {
	driver, ok := host.(*Driver)
	if !ok || driver == nil || driver.closed || driver.device.IsUndefined() || driver.device.IsNull() {
		return platform.ErrFeatureInvalid
	}

	if driver.particleBridge != nil {
		return platform.ErrFeatureDuplicate
	}

	driver.particleBridge = &particleFeature{Driver: driver}

	return nil
}

func (driver *Driver) PrepareParticles(instances []*particledata.Instance) error {
	if driver.particleBridge == nil {
		return platform.ErrFeatureMissing
	}

	return driver.particleBridge.Prepare(instances)
}

func (feature *particleFeature) Prepare(instances []*particledata.Instance) (err error) {
	feature.device.Call("pushErrorScope", "out-of-memory")
	feature.device.Call("pushErrorScope", "validation")

	defer func() {
		for range 2 {
			result, scopeErr := awaitPromise(feature.device.Call("popErrorScope"))
			if scopeErr != nil {
				err = errors.Join(err, scopeErr)
			} else if !result.IsNull() && !result.IsUndefined() {
				err = errors.Join(err, fmt.Errorf("%w: %s", particledata.ErrGPU, result.Get("message").String()))
			}
		}
	}()

	feature.pages = make(map[*uipaint.Texture]uiPage)
	feature.configs = make(map[*particledata.Layer]js.Value)
	feature.warmBuffers = make(map[*particledata.Layer]js.Value)
	feature.upload = js.Global().Get("Uint8Array").New(particledata.FrameBytes)
	feature.passAttachment = jsObject(map[string]any{loadOpProperty: particleLoad, storeOpProperty: uiStore})
	feature.passDescriptor = jsObject(map[string]any{uiAttachments: []any{feature.passAttachment}})

	if err := feature.prepareParticlePipelines(particledata.DrawSpaces(instances)); err != nil {
		return fmt.Errorf("prepare particle pipelines: %w", err)
	}

	for _, instance := range instances {
		effect := instance.Effect

		page, ok := feature.pages[effect.Page]
		if !ok {
			page, err = feature.uploadPreparedPage(effect.Page, feature.pipelines[instance.Space()][0], feature.sampler)
			if err != nil {
				return err
			}

			feature.pages[effect.Page] = page
		}

		for index := range effect.Layers {
			source := &effect.Layers[index]

			state := feature.prepareLayer(source, page.group, instance.Space())
			if len(source.Warm) != 0 {
				feature.prepareWarm(source, state, &instance.Steps()[index])
			}
		}
	}

	return nil
}

func (feature *particleFeature) Simulate(encoder js.Value, instances []*particledata.Instance) {
	for _, warm := range feature.warmCopies {
		if warm.step.Reset {
			encoder.Call("copyParticleState", warm.source, warm.destination, warm.size)
		}
	}

	var begun bool

	pass := feature.computePass
	feature.screenActive = false
	index := 0

	for _, instance := range instances {
		steps := instance.Steps()
		for i := range steps {
			step := &steps[i]
			layer := &feature.layers[index]
			index++
			layer.active, layer.slots = step.Active, step.Slots

			feature.screenActive = feature.screenActive || (layer.space == particledata.ScreenSpace && step.Active)
			if !step.Active && !step.Reset {
				continue
			}

			js.CopyBytesToJS(feature.upload, step.Bytes[:])
			feature.queue.Call("writeBuffer", layer.frame, 0, feature.upload)

			if !begun {
				encoder.Call("beginComputePass")

				begun = true
			}

			count := layer.slots
			if step.Reset {
				count = layer.capacity
			}

			pass.Call("setPipeline", feature.advance)
			pass.Call("setBindGroup", 0, layer.computeGroup)
			pass.Call("dispatchWorkgroups", (count+particledata.WorkgroupSize-1)/particledata.WorkgroupSize)
			feature.metrics.Dispatch()

			if step.Births != 0 || step.Reset {
				pass.Call("setPipeline", feature.spawn)
				pass.Call("dispatchWorkgroups", 1)
				feature.metrics.Dispatch()
			}
		}
	}

	if begun {
		pass.Call("end")
	}
}

func (feature *particleFeature) Draw(encoder, view js.Value) {
	if !feature.screenActive {
		return
	}

	feature.passAttachment.Set(viewProperty, view)
	pass := feature.startRenderPass(encoder, feature.passDescriptor)

	feature.drawLayers(pass, particledata.ScreenSpace)

	pass.Call("end")
	feature.passAttachment.Set(viewProperty, js.Undefined())
}

func (feature *particleFeature) DrawWorld(pass js.Value, projection [16]float32) {
	if feature.camera.Update(projection) {
		js.CopyBytesToJS(feature.cameraUpload, feature.camera.Bytes[:])
		feature.queue.Call("writeBuffer", feature.cameraBuffer, 0, feature.cameraUpload)
	}

	pass.Call("setBindGroup", particledata.CameraGroup, feature.cameraGroup)
	feature.drawLayers(pass, particledata.WorldSpace)
}

func (feature *particleFeature) Close() {
	for _, layer := range feature.layers {
		layer.state.Call("destroy")
		layer.frame.Call("destroy")
	}

	for _, config := range feature.configs {
		config.Call("destroy")
	}

	for _, buffer := range feature.warmBuffers {
		buffer.Call("destroy")
	}

	for _, page := range feature.pages {
		page.texture.Call("destroy")
	}

	if !feature.cameraBuffer.IsUndefined() {
		feature.cameraBuffer.Call("destroy")
	}

	feature.layers, feature.configs, feature.pages = nil, nil, nil
	feature.warmBuffers, feature.warmCopies = nil, nil
}

func (feature *particleFeature) drawLayers(pass js.Value, space particledata.Space) {
	for _, layer := range feature.layers {
		if layer.active && layer.space == space {
			pass.Call("setPipeline", feature.pipelines[space][layer.blend])
			pass.Call("setBindGroup", 0, layer.drawGroup)
			pass.Call("setBindGroup", 1, layer.page)
			pass.Call("setVertexBufferRange", 0, layer.state, particledata.StateHeader, layer.slots*particledata.ParticleBytes)
			pass.Call("draw", particledata.QuadVertices, layer.slots)
			feature.metrics.DrawSprites(particledata.QuadVertices, uint64(layer.slots))
		}
	}
}

func (feature *particleFeature) prepareParticlePipelines(spaces [particledata.Spaces]bool) error {
	device := feature.device
	module := device.Call("createShaderModule", jsObject(map[string]any{shaderCodeProperty: particledata.Shader}))
	feature.computeLayout = device.Call("createBindGroupLayout", jsObject(map[string]any{
		entriesProperty: particleBufferLayout(4, true),
	}))
	computeLayout := device.Call("createPipelineLayout", jsObject(map[string]any{
		particleLayoutsProperty: []any{feature.computeLayout},
	}))

	var err error

	feature.advance, err = feature.createParticleCompute(module, computeLayout, "advance")
	if err != nil {
		return err
	}

	feature.spawn, err = feature.createParticleCompute(module, computeLayout, "spawn")
	if err != nil {
		return err
	}

	return feature.prepareParticleDraw(module, spaces)
}

func (feature *particleFeature) createParticleCompute(module, layout js.Value, entry string) (js.Value, error) {
	return awaitPromise(feature.device.Call("createComputePipelineAsync", jsObject(map[string]any{
		layoutProperty: layout, particleComputeProperty: jsObject(map[string]any{moduleProperty: module, entryPointProperty: entry}),
	})))
}

func particleVertexLayout() []any {
	return []any{jsObject(map[string]any{
		arrayStrideProperty: particledata.ParticleBytes, stepModeProperty: uiInstanceStep,
		attributesProperty: []any{
			jsObject(map[string]any{formatProperty: formatFloat32x3, offsetProperty: 0, shaderLocationProperty: 0}),
			jsObject(
				map[string]any{formatProperty: uiFloat4Format, offsetProperty: particledata.ParticleAgeOffset, shaderLocationProperty: 1},
			),
			jsObject(
				map[string]any{
					formatProperty:         formatFloat32x3,
					offsetProperty:         particledata.VelocityOffset,
					shaderLocationProperty: particledata.VelocityLocation,
				},
			),
		},
	})}
}

func particleBufferLayout(visibility int, compute bool) []any {
	buffer := func(binding int, kind string, size int) js.Value {
		return jsObject(map[string]any{
			bindingProperty: binding, particleVisibilityProperty: visibility,
			gpuBufferProperty: jsObject(map[string]any{bindingTypeProperty: kind, particleMinBindingProperty: size}),
		})
	}

	entries := []any{buffer(1, "uniform", particledata.ConfigBytes), buffer(particledata.FrameBinding, "uniform", particledata.FrameBytes)}
	if compute {
		entries = append(entries, buffer(0, "storage", 0))
	}

	return entries
}

func (feature *particleFeature) prepareLayer(source *particledata.Layer, page js.Value, space particledata.Space) js.Value {
	config, exists := feature.configs[source]
	if !exists {
		config = feature.particleBuffer("Prepared particle curves", particledata.ConfigBytes, 0x48)
		array := js.Global().Get("Uint8Array").New(particledata.ConfigBytes)
		js.CopyBytesToJS(array, source.Config)
		feature.queue.Call("writeBuffer", config, 0, array)
		feature.configs[source] = config
	}

	const stateUsage = 0xa8 // Storage, vertex, copy destination.

	stateSize := particledata.StateHeader + int(source.Capacity)*particledata.ParticleBytes
	state := feature.particleBuffer("Particle slots", stateSize, stateUsage)
	frame := feature.particleBuffer("Particle step", particledata.FrameBytes, 0x48)
	feature.layers = append(feature.layers, particleLayer{
		state: state, frame: frame, capacity: source.Capacity, blend: source.Blend, page: page, space: space,
		computeGroup: feature.particleGroup(feature.computeLayout, config, frame, state),
		drawGroup:    feature.particleGroup(feature.drawLayout, config, frame, js.Undefined()),
	})

	return state
}

func (feature *particleFeature) prepareWarm(source *particledata.Layer, state js.Value, step *particledata.Step) {
	const warmUsage = 0x0c // Copy source and destination.

	buffer, exists := feature.warmBuffers[source]
	if !exists {
		buffer = feature.particleBuffer("Prepared warm particle state", len(source.Warm), warmUsage)
		array := js.Global().Get("Uint8Array").New(len(source.Warm))
		js.CopyBytesToJS(array, source.Warm)
		feature.queue.Call("writeBuffer", buffer, 0, array)
		feature.warmBuffers[source] = buffer
	}

	feature.warmCopies = append(feature.warmCopies, particleWarmCopy{
		source: buffer, destination: state, step: step, size: len(source.Warm),
	})
}

func (feature *particleFeature) particleBuffer(label string, size, usage int) js.Value {
	return feature.device.Call("createBuffer", jsObject(map[string]any{
		labelProperty: label, sizeProperty: size, usageProperty: usage,
	}))
}

func (feature *particleFeature) particleGroup(layout, config, frame, state js.Value) js.Value {
	binding := func(slot int, buffer js.Value) js.Value {
		return jsObject(map[string]any{
			bindingProperty: slot, resourceProperty: jsObject(map[string]any{gpuBufferProperty: buffer}),
		})
	}

	entries := []any{binding(1, config), binding(particledata.FrameBinding, frame)}
	if !state.IsUndefined() {
		entries = append(entries, binding(0, state))
	}

	return feature.device.Call("createBindGroup", jsObject(map[string]any{
		layoutProperty: layout, entriesProperty: entries,
	}))
}

func (driver *Driver) simulateParticles(encoder js.Value, instances []*particledata.Instance) {
	if driver.particleBridge != nil {
		driver.particleBridge.Simulate(encoder, instances)
	}
}

func (driver *Driver) closeParticles() {
	if driver.particleBridge != nil {
		driver.particleBridge.Close()
		driver.particleBridge = nil
	}
}
