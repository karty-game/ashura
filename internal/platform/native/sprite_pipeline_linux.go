//go:build linux

package native

import (
	"github.com/gogpu/gputypes"
	"github.com/karty-game/ashura/internal/spritepaint"
	wgpu "github.com/karty-game/ashura/internal/webgpu"
)

const (
	spriteViewportSize = 16
	spriteVertices     = 6
)

func (driver *spriteFeature) prepareSprites() error {
	if driver.sprites != nil {
		return nil
	}

	shader, err := driver.device.CreateShaderModuleWGSL(spritepaint.Shader)
	if err != nil {
		return err
	}
	defer shader.Release()

	blend := wgpu.BlendComponent{
		Operation: gputypes.BlendOperationAdd,
		SrcFactor: gputypes.BlendFactorOne,
		DstFactor: gputypes.BlendFactorOneMinusSrcAlpha,
	}

	pipeline, err := driver.device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
		Label:  "Screen2D pixel sprites",
		Vertex: wgpu.VertexState{Module: shader, EntryPoint: "vertexMain", Buffers: spriteVertexLayout()},
		Primitive: wgpu.PrimitiveState{
			Topology: gputypes.PrimitiveTopologyTriangleList, FrontFace: gputypes.FrontFaceCCW, CullMode: gputypes.CullModeNone,
		},
		Multisample: wgpu.MultisampleState{Count: 1, Mask: allSampleMask},
		Fragment: &wgpu.FragmentState{
			Module: shader, EntryPoint: "fragmentMain",
			Targets: []wgpu.ColorTargetState{
				{Format: driver.format, WriteMask: gputypes.ColorWriteMaskAll, Blend: &wgpu.BlendState{Color: blend, Alpha: blend}},
			},
		},
	})
	if err != nil {
		return err
	}

	gpu := &spriteGPU{pipeline: pipeline, pages: make(map[*spritepaint.Texture]uiPage)}
	gpu.passColors[0] = wgpu.RenderPassColorAttachment{LoadOp: gputypes.LoadOpLoad, StoreOp: gputypes.StoreOpStore}
	gpu.passDescriptor.ColorAttachments = gpu.passColors[:]
	driver.sprites = gpu

	gpu.uniform, err = driver.device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "Screen2D viewport", Size: spriteViewportSize, Usage: wgpu.BufferUsageUniform | wgpu.BufferUsageCopyDst,
	})
	if err != nil {
		driver.closeSprites()

		return err
	}

	gpu.sampler, err = driver.device.CreateSampler(&wgpu.SamplerDescriptor{
		AddressModeU: gputypes.AddressModeClampToEdge,
		AddressModeV: gputypes.AddressModeClampToEdge,
		AddressModeW: gputypes.AddressModeClampToEdge,
		MinFilter:    gputypes.FilterModeNearest,
		MagFilter:    gputypes.FilterModeNearest,
	})
	if err != nil {
		driver.closeSprites()
	}

	return err
}

func spriteVertexLayout() []wgpu.VertexBufferLayout {
	const (
		vectorBytes    = 16
		uvLocation     = 2
		attributeCount = 3
	)

	attributes := []wgpu.VertexAttribute{
		{Format: gputypes.VertexFormatFloat32x4, Offset: 0, ShaderLocation: 0},
		{Format: gputypes.VertexFormatFloat32x4, Offset: vectorBytes, ShaderLocation: 1},
		{Format: gputypes.VertexFormatFloat32x4, Offset: uvLocation * vectorBytes, ShaderLocation: uvLocation},
	}

	return []wgpu.VertexBufferLayout{
		{
			ArrayStride:    spritepaint.Stride,
			StepMode:       gputypes.VertexStepModeInstance,
			AttributeCount: attributeCount,
			Attributes:     &attributes[0],
		},
	}
}
