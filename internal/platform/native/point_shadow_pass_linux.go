//go:build linux

package native

import (
	"fmt"

	"github.com/gogpu/gputypes"
	"github.com/karty-game/ashura/internal/render"
	wgpu "github.com/karty-game/ashura/internal/webgpu"
)

func (driver *pointShadowFeature) initPointShadowPasses(shadow *pointShadowGPU) error {
	if shadow.pipelines[0] != nil {
		return nil
	}

	var err error
	for face := range render.PointShadowFaces {
		shadow.faces[face], err = shadow.image.CreateView(
			&wgpu.TextureViewDescriptor{
				Dimension:       gputypes.TextureViewDimension2D,
				MipLevelCount:   1,
				BaseArrayLayer:  uint32(face),
				ArrayLayerCount: 1,
			},
		)
		if err != nil {
			return err
		}

		shadow.uniforms[face], err = driver.device.CreateBuffer(
			&wgpu.BufferDescriptor{Usage: gputypes.BufferUsageUniform | gputypes.BufferUsageCopyDst, Size: render.PointShadowFaceSize},
		)
		if err != nil {
			return err
		}
	}

	shader, err := driver.device.CreateShaderModuleWGSL(render.ShadowDepthShader())
	if err != nil {
		return err
	}
	defer shader.Release()

	for variant := range 2 {
		if err = driver.createPointShadowVariant(shadow, shader, variant); err != nil {
			return err
		}
	}

	return nil
}

//nolint:mnd // Attribute offsets and locations encode the admitted mesh/instance ABI.
func (driver *pointShadowFeature) createPointShadowVariant(shadow *pointShadowGPU, shader *wgpu.ShaderModule, variant int) error {
	cull := gputypes.CullModeNone
	if variant == 1 {
		cull = gputypes.CullModeBack
	}

	attributes := []wgpu.VertexAttribute{{Format: gputypes.VertexFormatFloat32x3, Offset: 0, ShaderLocation: 0}}
	models := []wgpu.VertexAttribute{
		{Format: gputypes.VertexFormatFloat32x4, Offset: 0, ShaderLocation: 3},
		{Format: gputypes.VertexFormatFloat32x4, Offset: 16, ShaderLocation: 4},
		{Format: gputypes.VertexFormatFloat32x4, Offset: 32, ShaderLocation: 5},
		{Format: gputypes.VertexFormatFloat32x4, Offset: 48, ShaderLocation: 6},
	}

	pipeline, err := driver.device.CreateRenderPipeline(
		&wgpu.RenderPipelineDescriptor{
			Label: "point shadow projected depth",
			Vertex: wgpu.VertexState{
				Module:     shader,
				EntryPoint: pointShadowVertexEntry,
				Buffers: []wgpu.VertexBufferLayout{
					{
						ArrayStride:    render.VertexStride,
						StepMode:       gputypes.VertexStepModeVertex,
						AttributeCount: uintptr(len(attributes)),
						Attributes:     &attributes[0],
					},
					{
						ArrayStride:    render.InstanceStride,
						StepMode:       gputypes.VertexStepModeInstance,
						AttributeCount: uintptr(len(models)),
						Attributes:     &models[0],
					},
				},
			},
			Primitive: wgpu.PrimitiveState{
				Topology:  gputypes.PrimitiveTopologyTriangleList,
				FrontFace: gputypes.FrontFaceCW,
				CullMode:  cull,
			},
			DepthStencil: &wgpu.DepthStencilState{
				Format:            gputypes.TextureFormatDepth32Float,
				DepthWriteEnabled: true,
				DepthCompare:      gputypes.CompareFunctionLess,
				DepthBias:         render.ShadowDepthBias, DepthBiasSlopeScale: render.ShadowDepthBiasSlopeScale,
			},
			Multisample: wgpu.MultisampleState{Count: 1, Mask: allSampleMask},
		},
	)
	if err != nil {
		return fmt.Errorf("create point shadow pipeline: %w", err)
	}

	shadow.pipelines[variant] = pipeline

	layout := pipeline.GetBindGroupLayout(0)
	defer layout.Release()

	for face := range render.PointShadowFaces {
		shadow.groups[variant][face], err = driver.device.CreateBindGroup(
			&wgpu.BindGroupDescriptor{
				Layout:  layout,
				Entries: []wgpu.BindGroupEntry{{Binding: 0, Buffer: shadow.uniforms[face], Size: render.PointShadowFaceSize}},
			},
		)
		if err != nil {
			return err
		}
	}

	return nil
}

func (driver *pointShadowFeature) recordPointShadow(encoder *wgpu.CommandEncoder, frame render.Frame) error {
	light, enabled := render.ShadowLight(frame)

	shadow := driver.pointShadow
	if shadow == nil || enabled && (shadow.pipelines[0] == nil || shadow.resolution != light.ShadowResolution) {
		return render.ErrPointShadowStartup
	}

	var dirty uint8
	if enabled {
		dirty = shadow.state.Update(frame, light)
	}

	data := shadow.state.SampleUniform(light, enabled)
	if data != shadow.sampleData {
		if err := driver.queue.WriteBuffer(shadow.sampleUniform, 0, data[:]); err != nil {
			shadow.state.Invalidate()

			return err
		}

		shadow.sampleData = data
	}

	if dirty == 0 {
		return nil
	}

	masks := shadow.state.FaceMasks()

	for face := range render.PointShadowFaces {
		if dirty&(1<<face) == 0 {
			continue
		}

		if err := driver.recordPointShadowFace(encoder, frame, masks, face); err != nil {
			shadow.state.Invalidate()

			return err
		}
	}

	return nil
}

func (driver *pointShadowFeature) recordPointShadowFace(
	encoder *wgpu.CommandEncoder, frame render.Frame, masks []uint8, face int,
) error {
	shadow := driver.pointShadow

	uniform := shadow.state.FaceUniform(face)
	if uniform != shadow.faceData[face] {
		if err := driver.queue.WriteBuffer(shadow.uniforms[face], 0, uniform[:]); err != nil {
			return err
		}

		shadow.faceData[face] = uniform
	}

	pass, err := encoder.BeginRenderPass(
		&wgpu.RenderPassDescriptor{
			Label: "point shadow face",
			DepthStencilAttachment: &wgpu.RenderPassDepthStencilAttachment{
				View: shadow.faces[face], DepthLoadOp: gputypes.LoadOpClear,
				DepthStoreOp: gputypes.StoreOpStore, DepthClearValue: 1,
			},
		},
	)
	if err != nil {
		return err
	}

	err = driver.drawPointShadowFace(pass, frame, masks, face)
	pass.End()
	pass.Release()

	return err
}

func (driver *pointShadowFeature) drawPointShadowFace(pass *wgpu.RenderPassEncoder, frame render.Frame, masks []uint8, face int) error {
	for index, draw := range frame.Draws {
		if masks[index]&(1<<face) == 0 {
			continue
		}

		geometry, err := driver.geometry.Resolve(draw, driver.createGeometry)
		if err != nil {
			return err
		}

		instances, err := driver.instanceBuffer(draw)
		if err != nil {
			return err
		}

		variant := 0
		if draw.CullBackfaces {
			variant = 1
		}

		pass.SetPipeline(driver.pointShadow.pipelines[variant])
		pass.SetBindGroup(0, driver.pointShadow.groups[variant][face], nil)
		pass.SetVertexBuffer(0, geometry.vertices, 0, geometry.vertexBytes)
		pass.SetVertexBuffer(1, instances.buffer, 0, uint64(len(instances.state.Data)))
		pass.SetIndexBuffer(geometry.indices, gputypes.IndexFormatUint32, 0, geometry.indexBytes)
		//nolint:gosec // Geometry and instance admission bound both nonnegative counts to uint32.
		pass.DrawIndexed(uint32(render.IndexCount(draw)), uint32(instances.state.Count()), 0, 0, 0)
	}

	return nil
}
