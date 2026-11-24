//go:build linux

package native

import (
	"github.com/gogpu/gputypes"
	"github.com/karty-game/ashura/internal/render"
	wgpu "github.com/karty-game/ashura/internal/webgpu"
)

const spotDepthLabel = "spot shadow depth"

func (lighting *gpuClusteredLights) prepareSpots(device *wgpu.Device) error {
	module, err := device.CreateShaderModuleWGSL(render.ShadowDepthShader())
	if err != nil {
		return err
	}
	defer module.Release()

	buffers := modelVertexBuffers(false, true)
	buffers[0].AttributeCount = 1

	lighting.spotPipeline, err = device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
		Label:     spotDepthLabel,
		Vertex:    wgpu.VertexState{Module: module, EntryPoint: pointShadowVertexEntry, Buffers: buffers},
		Primitive: wgpu.PrimitiveState{Topology: gputypes.PrimitiveTopologyTriangleList, CullMode: gputypes.CullModeNone},
		DepthStencil: &wgpu.DepthStencilState{
			Format: gputypes.TextureFormatDepth32Float, DepthWriteEnabled: true, DepthCompare: gputypes.CompareFunctionLess,
			DepthBias: render.ShadowDepthBias, DepthBiasSlopeScale: render.ShadowDepthBiasSlopeScale,
		},
		Multisample: wgpu.MultisampleState{Count: 1, Mask: allSampleMask},
	})
	if err != nil {
		return err
	}

	layout := lighting.spotPipeline.GetBindGroupLayout(0)
	defer layout.Release()

	for slot := range lighting.spotTargets {
		if err := lighting.spotTargets[slot].init(device, layout, render.SpotShadowResolution); err != nil {
			return err
		}
	}

	return nil
}

func (target *gpuSpotTarget) init(device *wgpu.Device, layout *wgpu.BindGroupLayout, resolution uint32) error {
	var err error

	target.image, err = device.CreateTexture(&wgpu.TextureDescriptor{
		Label: spotDepthLabel, Format: gputypes.TextureFormatDepth32Float,
		Usage:         gputypes.TextureUsageRenderAttachment | gputypes.TextureUsageTextureBinding,
		Dimension:     gputypes.TextureDimension2D,
		Size:          gputypes.Extent3D{Width: resolution, Height: resolution, DepthOrArrayLayers: 1},
		MipLevelCount: 1, SampleCount: 1,
	})
	if err != nil {
		return err
	}

	target.view, err = target.image.CreateView(nil)
	if err != nil {
		return err
	}

	target.uniform, err = device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "spot shadow matrix", Size: render.SpotMatrixBytes,
		Usage: gputypes.BufferUsageUniform | gputypes.BufferUsageCopyDst,
	})
	if err != nil {
		return err
	}

	target.group, err = device.CreateBindGroup(&wgpu.BindGroupDescriptor{
		Layout: layout, Entries: []wgpu.BindGroupEntry{{Binding: 0, Buffer: target.uniform, Size: render.SpotMatrixBytes}},
	})

	return err
}

func (driver *Driver) drawSpotShadows(encoder *wgpu.CommandEncoder, frame render.Frame, dirty uint8, masks []uint8) error {
	lighting := driver.clusteredLights
	for slot := range lighting.spotTargets {
		bit := uint8(1 << slot)
		if dirty&bit == 0 {
			continue
		}

		target := &lighting.spotTargets[slot]

		matrix := lighting.spots.Views[slot].Uniform()
		if matrix != target.matrix {
			if err := driver.queue.WriteBuffer(target.uniform, 0, matrix[:]); err != nil {
				return err
			}

			target.matrix = matrix
		}

		pass, err := encoder.BeginRenderPass(&wgpu.RenderPassDescriptor{
			Label: spotDepthLabel,
			DepthStencilAttachment: &wgpu.RenderPassDepthStencilAttachment{
				View: target.view, DepthLoadOp: gputypes.LoadOpClear, DepthStoreOp: gputypes.StoreOpStore, DepthClearValue: 1,
			},
		})
		if err != nil {
			return err
		}

		pass.SetPipeline(lighting.spotPipeline)
		pass.SetBindGroup(0, target.group, nil)
		err = driver.drawSpotCasters(pass, frame, masks, bit)
		pass.End()
		pass.Release()

		if err != nil {
			return err
		}
	}

	return nil
}

func (driver *Driver) drawSpotCasters(pass *wgpu.RenderPassEncoder, frame render.Frame, masks []uint8, bit uint8) error {
	for index, draw := range frame.Draws {
		if masks[index]&bit == 0 {
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

		pass.SetVertexBuffer(0, geometry.vertices, 0, geometry.vertexBytes)
		pass.SetVertexBuffer(1, instances.buffer, 0, uint64(len(instances.state.Data)))
		pass.SetIndexBuffer(geometry.indices, gputypes.IndexFormatUint32, 0, geometry.indexBytes)
		//nolint:gosec // Geometry and instance admission bound both counts.
		pass.DrawIndexed(uint32(render.IndexCount(draw)), uint32(instances.state.Count()), 0, 0, 0)
	}

	return nil
}

func (target *gpuSpotTarget) release() {
	if target.group != nil {
		target.group.Release()
	}

	if target.uniform != nil {
		target.uniform.Release()
	}

	if target.view != nil {
		target.view.Release()
	}

	if target.image != nil {
		target.image.Release()
	}
}
