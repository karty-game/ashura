//go:build linux

package native

import (
	"fmt"

	"github.com/gogpu/gputypes"
	"github.com/karty-game/ashura/internal/render"
	wgpu "github.com/karty-game/ashura/internal/webgpu"
)

type gpuGeometry struct {
	vertices, indices       *wgpu.Buffer
	lightmapUV              *wgpu.Buffer
	lightmapBytes           uint64
	vertexBytes, indexBytes uint64
}

func (geometry gpuGeometry) release() {
	geometry.vertices.Release()
	geometry.indices.Release()

	if geometry.lightmapUV != nil {
		geometry.lightmapUV.Release()
	}
}

func (driver *Driver) createGeometry(vertices, indices, lightmapUV []byte) (gpuGeometry, error) {
	vertexBuffer, err := createBuffer(driver, vertices, gputypes.BufferUsageVertex)
	if err != nil {
		return gpuGeometry{}, err
	}

	indexBuffer, err := createBuffer(driver, indices, gputypes.BufferUsageIndex)
	if err != nil {
		vertexBuffer.Release()

		return gpuGeometry{}, err
	}

	result := gpuGeometry{
		vertices:    vertexBuffer,
		indices:     indexBuffer,
		vertexBytes: uint64(len(vertices)),
		indexBytes:  uint64(len(indices)),
	}
	if len(lightmapUV) != 0 {
		result.lightmapUV, err = createBuffer(driver, lightmapUV, gputypes.BufferUsageVertex)
		if err != nil {
			result.release()

			return gpuGeometry{}, err
		}

		result.lightmapBytes = uint64(len(lightmapUV))
	}

	return result, nil
}

func drawCalls(driver *Driver, pass *wgpu.RenderPassEncoder, frame render.Frame) error {
	if err := driver.prepareDecals(frame.Decals); err != nil {
		return err
	}

	if err := driver.updateFrameUniform(frame); err != nil {
		return err
	}

	var active *wgpu.RenderPipeline

	order := driver.opaqueOrder.Prepare(frame)
	for index := range frame.Draws {
		if order != nil {
			index = order[index]
		}

		draw := frame.Draws[index]
		if !render.HasGeometry(draw) {
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

		group, err := driver.materialGroup(draw)
		if err != nil {
			return err
		}

		pipeline := driver.drawPipeline(draw)
		if pipeline != active {
			pass.SetPipeline(pipeline)
			driver.bindPointShadow(pass, draw)
			driver.bindDecals(pass, draw)

			active = pipeline
		}

		pass.SetBindGroup(0, group, nil)
		pass.SetVertexBuffer(0, geometry.vertices, 0, geometry.vertexBytes)
		pass.SetVertexBuffer(1, instances.buffer, 0, uint64(len(instances.state.Data)))

		if draw.Impostor == nil {
			if geometry.lightmapUV != nil {
				pass.SetVertexBuffer(2, geometry.lightmapUV, 0, geometry.lightmapBytes)
			} else {
				pass.SetVertexBuffer(2, geometry.vertices, 0, geometry.vertexBytes)
			}
		}

		pass.SetIndexBuffer(geometry.indices, gputypes.IndexFormatUint32, 0, geometry.indexBytes)

		indexCount := uint32(render.IndexCount(draw))    //nolint:gosec // Full geometry admission bounds this nonnegative count to uint32.
		instanceCount := uint32(instances.state.Count()) //nolint:gosec // Instance admission bounds this nonnegative count to 128MiB/64.
		pass.DrawIndexed(indexCount, instanceCount, 0, 0, 0)
		driver.metrics.Draw(uint64(indexCount), uint64(instanceCount))
	}

	return nil
}

func createBuffer(driver *Driver, data []byte, usage gputypes.BufferUsage) (*wgpu.Buffer, error) {
	buffer, err := driver.device.CreateBuffer(&wgpu.BufferDescriptor{
		Usage: usage | gputypes.BufferUsageCopyDst,
		Size:  uint64(len(data)),
	})
	if err != nil {
		return nil, fmt.Errorf("create draw buffer: %w", err)
	}

	if err := driver.queue.WriteBuffer(buffer, 0, data); err != nil {
		buffer.Release()

		return nil, fmt.Errorf("upload draw buffer: %w", err)
	}

	return buffer, nil
}

func (driver *Driver) beginScenePass(
	encoder *wgpu.CommandEncoder,
	view *wgpu.TextureView,
	clearColor [4]float32,
) (*wgpu.RenderPassEncoder, error) {
	driver.passColors[0] = driver.colorAttachment(view, clearColor)
	if driver.passDescriptor.DepthStencilAttachment == nil {
		driver.passDepth = wgpu.RenderPassDepthStencilAttachment{
			DepthLoadOp:     gputypes.LoadOpClear,
			DepthStoreOp:    gputypes.StoreOpDiscard,
			DepthClearValue: 1,
		}
		driver.passDescriptor.ColorAttachments = driver.passColors[:]
		driver.passDescriptor.DepthStencilAttachment = &driver.passDepth
	}

	driver.passDepth.View = driver.depthView

	return encoder.BeginRenderPass(&driver.passDescriptor)
}

func (driver *Driver) drawScenePass(encoder *wgpu.CommandEncoder, view *wgpu.TextureView, frame render.Frame) error {
	defer driver.geometry.Prune(gpuGeometry.release)
	defer driver.pruneInstances()

	if len(frame.Draws) == 0 && !frame.WorldParticles {
		return driver.clearSurface(encoder, view, frame.ClearColor)
	}

	if err := driver.resizeTargets(); err != nil {
		return err
	}

	pass, err := driver.beginScenePass(encoder, view, frame.ClearColor)
	if err != nil {
		return fmt.Errorf("begin render pass: %w", err)
	}

	err = driver.drawSceneContents(pass, frame)

	pass.End()
	pass.Release()

	if err != nil {
		return err
	}

	return nil
}

func (driver *Driver) drawSceneContents(pass *wgpu.RenderPassEncoder, frame render.Frame) error {
	if len(frame.Draws) > 0 {
		if err := drawCalls(driver, pass, frame); err != nil {
			return err
		}
	}

	if frame.WorldParticles {
		if err := driver.particleBridge.DrawWorld(pass, frame.ViewProjection); err != nil {
			return err
		}
	}

	driver.drawPointShadowDebug(pass, frame)

	return nil
}
