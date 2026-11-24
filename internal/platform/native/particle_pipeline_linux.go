//go:build linux

package native

import (
	"github.com/gogpu/gputypes"
	"github.com/karty-game/ashura/internal/particledata"
	wgpu "github.com/karty-game/ashura/internal/webgpu"
)

func (feature *particleFeature) prepareParticleDraw(module *wgpu.ShaderModule, spaces [particledata.Spaces]bool) error {
	device := feature.device

	var err error

	feature.drawLayout, err = device.CreateBindGroupLayoutSimple(particleBufferLayout(gputypes.ShaderStageVertex, false))
	if err != nil {
		return err
	}

	texture, err := device.CreateBindGroupLayoutSimple([]wgpu.BindGroupLayoutEntry{
		{Binding: 0, Visibility: gputypes.ShaderStageFragment, Texture: &wgpu.TextureBindingLayout{
			SampleType: gputypes.TextureSampleTypeFloat, ViewDimension: gputypes.TextureViewDimension2D,
		}},
		{
			Binding:    1,
			Visibility: gputypes.ShaderStageFragment,
			Sampler:    &wgpu.SamplerBindingLayout{Type: gputypes.SamplerBindingTypeFiltering},
		},
	})
	if err != nil {
		return err
	}
	defer texture.Release()

	for space, needed := range spaces {
		if needed {
			if err := feature.prepareSpace(module, texture, particledata.Space(space)); err != nil {
				return err
			}
		}
	}

	feature.sampler, err = device.CreateSampler(&wgpu.SamplerDescriptor{
		AddressModeU: gputypes.AddressModeClampToEdge, AddressModeV: gputypes.AddressModeClampToEdge,
		AddressModeW: gputypes.AddressModeClampToEdge, MinFilter: gputypes.FilterModeLinear, MagFilter: gputypes.FilterModeLinear,
	})

	return err
}

func (feature *particleFeature) prepareSpace(module *wgpu.ShaderModule, texture *wgpu.BindGroupLayout, space particledata.Space) error {
	device := feature.device
	layouts := []*wgpu.BindGroupLayout{feature.drawLayout, texture}
	entry, samples := "particleVertex", uint32(1)

	var depth *wgpu.DepthStencilState

	if space == particledata.WorldSpace {
		camera, err := feature.prepareParticleCamera()
		if err != nil {
			return err
		}
		defer camera.Release()

		layouts = append(layouts, camera)
		entry, samples = "worldParticleVertex", feature.sampleCount
		depth = &wgpu.DepthStencilState{Format: gputypes.TextureFormatDepth24Plus, DepthCompare: gputypes.CompareFunctionLess}
	}

	draw, err := device.CreatePipelineLayoutSimple(layouts)
	if err != nil {
		return err
	}
	defer draw.Release()

	for index := range feature.pipelines[space] {
		over := wgpu.BlendComponent{Operation: gputypes.BlendOperationAdd,
			SrcFactor: gputypes.BlendFactorOne, DstFactor: gputypes.BlendFactorOneMinusSrcAlpha}

		color := over
		if particledata.Blend(index) == particledata.Add {
			color.DstFactor = gputypes.BlendFactorOne
		}

		feature.pipelines[space][index], err = device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
			Label:        entry,
			Layout:       draw,
			Vertex:       wgpu.VertexState{Module: module, EntryPoint: entry, Buffers: particleVertexLayout()},
			Primitive:    wgpu.PrimitiveState{Topology: gputypes.PrimitiveTopologyTriangleList},
			Multisample:  wgpu.MultisampleState{Count: samples, Mask: allSampleMask},
			DepthStencil: depth,
			Fragment: &wgpu.FragmentState{Module: module, EntryPoint: "particleFragment", Targets: []wgpu.ColorTargetState{
				{Format: feature.format, WriteMask: gputypes.ColorWriteMaskAll, Blend: &wgpu.BlendState{Color: color, Alpha: over}},
			}},
		})
		if err != nil {
			return err
		}
	}

	return nil
}

func (feature *particleFeature) prepareParticleCamera() (*wgpu.BindGroupLayout, error) {
	layout, err := feature.device.CreateBindGroupLayoutSimple([]wgpu.BindGroupLayoutEntry{{
		Binding: 0, Visibility: gputypes.ShaderStageVertex,
		Buffer: &wgpu.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform, MinBindingSize: particledata.CameraBytes},
	}})
	if err != nil {
		return nil, err
	}

	feature.cameraBuffer, err = feature.device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "Particle camera", Size: particledata.CameraBytes, Usage: wgpu.BufferUsageUniform | wgpu.BufferUsageCopyDst,
	})
	if err == nil {
		feature.cameraGroup, err = feature.device.CreateBindGroupSimple(layout, []wgpu.BindGroupEntry{{
			Binding: 0, Buffer: feature.cameraBuffer, Size: particledata.CameraBytes,
		}})
	}

	if err != nil {
		layout.Release()

		return nil, err
	}

	return layout, nil
}
