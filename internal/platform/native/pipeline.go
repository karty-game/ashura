//go:build linux

package native

import (
	"fmt"

	"github.com/gogpu/gputypes"
	"github.com/karty-game/karty-webgpu/internal/render"
	wgpu "github.com/karty-game/karty-webgpu/internal/webgpu"
)

func (driver *Driver) materialPipeline(source, cull bool) *wgpu.RenderPipeline {
	if source {
		if cull {
			return driver.sourceCulledPipeline
		}

		return driver.sourcePipeline
	}

	if cull {
		return driver.standardCulledPipeline
	}

	return driver.standardPipeline
}

func createPipeline(driver *Driver, shaderName, code string, source bool) error {
	shader, err := driver.device.CreateShaderModuleWGSL(code)
	if err != nil {
		return fmt.Errorf("create %s shader: %w", shaderName, err)
	}
	defer shader.Release()

	for _, cull := range []bool{false, true} {
		if err := createPipelineVariant(driver, shaderName, source, cull, false, shader); err != nil {
			return err
		}
	}

	return nil
}

func createPipelineVariant(driver *Driver, shaderName string, source, cull, impostor bool, shader *wgpu.ShaderModule) error {
	cullMode := gputypes.CullModeNone
	if cull {
		cullMode = gputypes.CullModeBack
	}

	pipeline, err := driver.device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
		Vertex: wgpu.VertexState{
			Module: shader, EntryPoint: pointShadowVertexEntry,
			Buffers: modelVertexBuffers(source, impostor),
		},
		Primitive: wgpu.PrimitiveState{
			Topology:  gputypes.PrimitiveTopologyTriangleList,
			FrontFace: gputypes.FrontFaceCCW,
			CullMode:  cullMode,
		},
		DepthStencil: &wgpu.DepthStencilState{
			Format:            gputypes.TextureFormatDepth24Plus,
			DepthWriteEnabled: true,
			DepthCompare:      gputypes.CompareFunctionLess,
		},
		Multisample: wgpu.MultisampleState{Count: driver.sampleCount, Mask: allSampleMask},
		Fragment: &wgpu.FragmentState{
			Module: shader, EntryPoint: render.FragmentEntry(
				driver.format != gputypes.TextureFormatBGRA8UnormSrgb && driver.format != gputypes.TextureFormatRGBA8UnormSrgb,
			),
			Targets: []wgpu.ColorTargetState{{Format: driver.format, WriteMask: gputypes.ColorWriteMaskAll}},
		},
	})
	if err != nil {
		return fmt.Errorf("create %s pipeline: %w", shaderName, err)
	}

	switch {
	case impostor:
		driver.impostorPipeline = pipeline
	case source && cull:
		driver.sourceCulledPipeline = pipeline
	case source:
		driver.sourcePipeline = pipeline
	case cull:
		driver.standardCulledPipeline = pipeline
	default:
		driver.standardPipeline = pipeline
	}

	return nil
}

func modelVertexBuffers(_, impostor bool) []wgpu.VertexBufferLayout {
	attributes := []wgpu.VertexAttribute{
		{Format: gputypes.VertexFormatFloat32x3, Offset: 0, ShaderLocation: 0},
		{Format: gputypes.VertexFormatFloat32x3, Offset: render.NormalOffset, ShaderLocation: 1},
		{Format: gputypes.VertexFormatFloat32x2, Offset: render.UVOffset, ShaderLocation: render.UVLocation},
	}
	//nolint:mnd // Offsets and shader locations encode the packed instance-matrix ABI.
	instanceAttributes := []wgpu.VertexAttribute{
		{Format: gputypes.VertexFormatFloat32x4, Offset: 0, ShaderLocation: 3},
		{Format: gputypes.VertexFormatFloat32x4, Offset: 16, ShaderLocation: 4},
		{Format: gputypes.VertexFormatFloat32x4, Offset: 32, ShaderLocation: 5},
		{Format: gputypes.VertexFormatFloat32x4, Offset: 48, ShaderLocation: 6},
	}

	buffers := []wgpu.VertexBufferLayout{
		{
			ArrayStride:    render.VertexStride,
			StepMode:       gputypes.VertexStepModeVertex,
			AttributeCount: uintptr(len(attributes)),
			Attributes:     &attributes[0],
		},
		{
			ArrayStride:    render.InstanceStride,
			StepMode:       gputypes.VertexStepModeInstance,
			AttributeCount: uintptr(len(instanceAttributes)),
			Attributes:     &instanceAttributes[0],
		},
	}

	lightmapAttribute := wgpu.VertexAttribute{Format: gputypes.VertexFormatFloat32x2, Offset: 0, ShaderLocation: render.LightmapUVLocation}
	if !impostor {
		buffers = append(
			buffers,
			wgpu.VertexBufferLayout{
				ArrayStride:    render.LightmapUVStride,
				StepMode:       gputypes.VertexStepModeVertex,
				AttributeCount: 1,
				Attributes:     &lightmapAttribute,
			},
		)
	}

	return buffers
}
