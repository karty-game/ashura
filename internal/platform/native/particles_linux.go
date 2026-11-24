//go:build linux

package native

import (
	"errors"
	"fmt"

	"github.com/gogpu/gputypes"
	"github.com/karty-game/ashura/internal/particledata"
	"github.com/karty-game/ashura/internal/platform"
	"github.com/karty-game/ashura/internal/uipaint"
	wgpu "github.com/karty-game/ashura/internal/webgpu"
)

type particleBackend interface {
	Prepare(instances []*particledata.Instance) error
	Simulate(encoder *wgpu.CommandEncoder, instances []*particledata.Instance) error
	Draw(encoder *wgpu.CommandEncoder, view *wgpu.TextureView) error
	DrawWorld(pass *wgpu.RenderPassEncoder, projection [16]float32) error
	Close()
}

type particleFeature struct {
	*Driver

	advance, spawn            *wgpu.ComputePipeline
	pipelines                 [particledata.Spaces][2]*wgpu.RenderPipeline
	camera                    particledata.Camera
	cameraBuffer              *wgpu.Buffer
	cameraGroup               *wgpu.BindGroup
	computeLayout, drawLayout *wgpu.BindGroupLayout
	sampler                   *wgpu.Sampler
	pages                     map[*uipaint.Texture]uiPage
	configs                   map[*particledata.Layer]*wgpu.Buffer
	warmBuffers               map[*particledata.Layer]*wgpu.Buffer
	warmCopies                []particleWarmCopy
	layers                    []particleLayer
	colors                    [1]wgpu.RenderPassColorAttachment
	pass                      wgpu.RenderPassDescriptor
	screenActive              bool
}

type particleLayer struct {
	state, frame            *wgpu.Buffer
	computeGroup, drawGroup *wgpu.BindGroup
	page                    *wgpu.BindGroup
	capacity                uint32
	slots                   uint32
	blend                   particledata.Blend
	active                  bool
	space                   particledata.Space
}

func RegisterParticles(host any) error {
	driver, ok := host.(*Driver)
	if !ok || driver == nil || driver.closed || driver.device == nil {
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
	feature.device.PushErrorScope(wgpu.ErrorFilterOutOfMemory)
	feature.device.PushErrorScope(wgpu.ErrorFilterValidation)

	defer func() {
		for range 2 {
			kind, message, scopeErr := feature.device.PopErrorScopeAsync(feature.instance)
			if scopeErr != nil {
				err = errors.Join(err, scopeErr)
			} else if kind != wgpu.ErrorTypeNoError {
				err = errors.Join(err, fmt.Errorf("%w: %s", particledata.ErrGPU, message))
			}
		}
	}()

	feature.pages = make(map[*uipaint.Texture]uiPage)
	feature.configs = make(map[*particledata.Layer]*wgpu.Buffer)
	feature.warmBuffers = make(map[*particledata.Layer]*wgpu.Buffer)
	feature.colors[0] = wgpu.RenderPassColorAttachment{LoadOp: gputypes.LoadOpLoad, StoreOp: gputypes.StoreOpStore}
	feature.pass.ColorAttachments = feature.colors[:]

	if err := feature.prepareParticlePipelines(particledata.DrawSpaces(instances)); err != nil {
		return fmt.Errorf("prepare particle pipelines: %w", err)
	}

	for _, instance := range instances {
		effect := instance.Effect

		page, ok := feature.pages[effect.Page]
		if !ok {
			var err error

			page, err = feature.uploadPreparedPage(effect.Page, feature.pipelines[instance.Space()][0], feature.sampler)
			if err != nil {
				return err
			}

			feature.pages[effect.Page] = page
		}

		for index := range effect.Layers {
			source := &effect.Layers[index]
			if err := feature.prepareLayer(source, page.group, instance.Space()); err != nil {
				return fmt.Errorf("prepare particle layer: %w", err)
			}

			if len(source.Warm) != 0 {
				state := feature.layers[len(feature.layers)-1].state
				if err := feature.prepareWarm(source, state, &instance.Steps()[index]); err != nil {
					return fmt.Errorf("prepare warm particle state: %w", err)
				}
			}
		}
	}

	return nil
}

func (feature *particleFeature) Simulate(encoder *wgpu.CommandEncoder, instances []*particledata.Instance) error {
	feature.copyWarmOnReset(encoder)

	var pass *wgpu.ComputePassEncoder
	defer func() {
		if pass != nil {
			pass.End()
			pass.Release()
		}
	}()

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

			if err := feature.queue.WriteBuffer(layer.frame, 0, step.Bytes[:]); err != nil {
				return err
			}

			if pass == nil {
				var err error

				pass, err = encoder.BeginComputePass(nil)
				if err != nil {
					return err
				}
			}

			count := layer.slots
			if step.Reset {
				count = layer.capacity
			}

			pass.SetPipeline(feature.advance)
			pass.SetBindGroup(0, layer.computeGroup, nil)
			pass.DispatchWorkgroups((count+particledata.WorkgroupSize-1)/particledata.WorkgroupSize, 1, 1)
			feature.metrics.Dispatch()

			if step.Births != 0 || step.Reset {
				pass.SetPipeline(feature.spawn)
				pass.DispatchWorkgroups(1, 1, 1)
				feature.metrics.Dispatch()
			}
		}
	}

	return nil
}

func (feature *particleFeature) Draw(encoder *wgpu.CommandEncoder, view *wgpu.TextureView) error {
	if !feature.screenActive {
		return nil
	}

	feature.colors[0].View = view

	pass, err := encoder.BeginRenderPass(&feature.pass)
	feature.colors[0].View = nil

	if err != nil {
		return err
	}

	defer pass.Release()
	defer pass.End()

	feature.drawLayers(pass, particledata.ScreenSpace)

	return nil
}

func (feature *particleFeature) DrawWorld(pass *wgpu.RenderPassEncoder, projection [16]float32) error {
	if feature.camera.Update(projection) {
		if err := feature.queue.WriteBuffer(feature.cameraBuffer, 0, feature.camera.Bytes[:]); err != nil {
			return err
		}
	}

	pass.SetBindGroup(particledata.CameraGroup, feature.cameraGroup, nil)
	feature.drawLayers(pass, particledata.WorldSpace)

	return nil
}

func (feature *particleFeature) Close() {
	for _, layer := range feature.layers {
		if layer.computeGroup != nil {
			layer.computeGroup.Release()
		}

		if layer.drawGroup != nil {
			layer.drawGroup.Release()
		}

		if layer.frame != nil {
			layer.frame.Release()
		}

		if layer.state != nil {
			layer.state.Release()
		}
	}

	for _, config := range feature.configs {
		config.Release()
	}

	for _, buffer := range feature.warmBuffers {
		buffer.Release()
	}

	for _, page := range feature.pages {
		page.group.Release()
		page.view.Release()
		page.texture.Release()
	}

	for _, pipelines := range feature.pipelines {
		for _, pipeline := range pipelines {
			if pipeline != nil {
				pipeline.Release()
			}
		}
	}

	if feature.cameraGroup != nil {
		feature.cameraGroup.Release()
	}

	if feature.cameraBuffer != nil {
		feature.cameraBuffer.Release()
	}

	if feature.advance != nil {
		feature.advance.Release()
	}

	if feature.spawn != nil {
		feature.spawn.Release()
	}

	if feature.computeLayout != nil {
		feature.computeLayout.Release()
	}

	if feature.drawLayout != nil {
		feature.drawLayout.Release()
	}

	if feature.sampler != nil {
		feature.sampler.Release()
	}
}

func (feature *particleFeature) drawLayers(pass *wgpu.RenderPassEncoder, space particledata.Space) {
	for _, layer := range feature.layers {
		if layer.active && layer.space == space {
			pass.SetPipeline(feature.pipelines[space][layer.blend])
			pass.SetBindGroup(0, layer.drawGroup, nil)
			pass.SetBindGroup(1, layer.page, nil)
			pass.SetVertexBuffer(0, layer.state, particledata.StateHeader, uint64(layer.slots)*particledata.ParticleBytes)
			pass.Draw(particledata.QuadVertices, layer.slots, 0, 0)
			feature.metrics.DrawSprites(particledata.QuadVertices, uint64(layer.slots))
		}
	}
}

func (feature *particleFeature) prepareParticlePipelines(spaces [particledata.Spaces]bool) error {
	device := feature.device

	module, err := device.CreateShaderModuleWGSL(particledata.Shader)
	if err != nil {
		return err
	}
	defer module.Release()

	feature.computeLayout, err = device.CreateBindGroupLayoutSimple(particleBufferLayout(gputypes.ShaderStageCompute, true))
	if err != nil {
		return err
	}

	compute, err := device.CreatePipelineLayoutSimple([]*wgpu.BindGroupLayout{feature.computeLayout})
	if err != nil {
		return err
	}
	defer compute.Release()

	feature.advance, err = device.CreateComputePipelineSimple(compute, module, "advance")
	if err != nil {
		return err
	}

	feature.spawn, err = device.CreateComputePipelineSimple(compute, module, "spawn")
	if err != nil {
		return err
	}

	return feature.prepareParticleDraw(module, spaces)
}

func particleVertexLayout() []wgpu.VertexBufferLayout {
	attributes := []wgpu.VertexAttribute{
		{Format: gputypes.VertexFormatFloat32x3, ShaderLocation: 0},
		{Format: gputypes.VertexFormatFloat32x4, ShaderLocation: 1, Offset: particledata.ParticleAgeOffset},
		{Format: gputypes.VertexFormatFloat32x3, ShaderLocation: particledata.VelocityLocation, Offset: particledata.VelocityOffset},
	}

	return []wgpu.VertexBufferLayout{{
		ArrayStride: particledata.ParticleBytes, StepMode: gputypes.VertexStepModeInstance,
		AttributeCount: uintptr(len(attributes)), Attributes: &attributes[0],
	}}
}

func particleBufferLayout(stage gputypes.ShaderStage, compute bool) []wgpu.BindGroupLayoutEntry {
	entries := []wgpu.BindGroupLayoutEntry{
		{
			Binding:    1,
			Visibility: stage,
			Buffer:     &wgpu.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform, MinBindingSize: particledata.ConfigBytes},
		},
		{
			Binding:    particledata.FrameBinding,
			Visibility: stage,
			Buffer:     &wgpu.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform, MinBindingSize: particledata.FrameBytes},
		},
	}
	if compute {
		entries = append(entries, wgpu.BindGroupLayoutEntry{
			Binding: 0, Visibility: stage, Buffer: &wgpu.BufferBindingLayout{Type: gputypes.BufferBindingTypeStorage},
		})
	}

	return entries
}

func (feature *particleFeature) prepareLayer(source *particledata.Layer, page *wgpu.BindGroup, space particledata.Space) error {
	feature.layers = append(feature.layers, particleLayer{capacity: source.Capacity, page: page, blend: source.Blend, space: space})
	gpu := &feature.layers[len(feature.layers)-1]

	config := feature.configs[source]

	var err error
	if config == nil {
		config, err = feature.device.CreateBuffer(&wgpu.BufferDescriptor{
			Label: "Prepared particle curves", Size: particledata.ConfigBytes, Usage: wgpu.BufferUsageUniform | wgpu.BufferUsageCopyDst,
		})
		if err != nil {
			return err
		}

		feature.configs[source] = config
		if err := feature.queue.WriteBuffer(config, 0, source.Config); err != nil {
			return err
		}
	}

	stateSize := uint64(particledata.StateHeader) + uint64(source.Capacity)*particledata.ParticleBytes

	gpu.state, err = feature.device.CreateBuffer(
		&wgpu.BufferDescriptor{
			Label: "Particle slots", Size: stateSize,
			Usage: wgpu.BufferUsageStorage | wgpu.BufferUsageVertex | wgpu.BufferUsageCopyDst,
		},
	)
	if err != nil {
		return err
	}

	gpu.frame, err = feature.device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "Particle step", Size: particledata.FrameBytes, Usage: wgpu.BufferUsageUniform | wgpu.BufferUsageCopyDst,
	})
	if err != nil {
		return err
	}

	entries := []wgpu.BindGroupEntry{
		{Binding: 0, Buffer: gpu.state, Size: stateSize},
		{Binding: 1, Buffer: config, Size: particledata.ConfigBytes},
		{Binding: particledata.FrameBinding, Buffer: gpu.frame, Size: particledata.FrameBytes},
	}

	gpu.computeGroup, err = feature.device.CreateBindGroupSimple(feature.computeLayout, entries)
	if err != nil {
		return err
	}

	gpu.drawGroup, err = feature.device.CreateBindGroupSimple(feature.drawLayout, entries[1:])

	return err
}

func (driver *Driver) simulateParticles(encoder *wgpu.CommandEncoder, instances []*particledata.Instance) error {
	if driver.particleBridge == nil {
		return nil
	}

	return driver.particleBridge.Simulate(encoder, instances)
}

func (driver *Driver) drawParticles(encoder *wgpu.CommandEncoder, view *wgpu.TextureView) error {
	if driver.particleBridge == nil {
		return nil
	}

	return driver.particleBridge.Draw(encoder, view)
}

func (driver *Driver) closeParticles() {
	if driver.particleBridge != nil {
		driver.particleBridge.Close()
		driver.particleBridge = nil
	}
}
