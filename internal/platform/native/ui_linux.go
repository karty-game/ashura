//go:build linux

//nolint:mnd,gosec // GPU ABI dimensions/counts come from bounded admitted UI records and prepared pages.
package native

import (
	"github.com/gogpu/gputypes"
	"github.com/karty-game/ashura/internal/uipaint"
	wgpu "github.com/karty-game/ashura/internal/webgpu"
)

type uiGPU struct {
	pipeline        *wgpu.RenderPipeline
	uniform, buffer *wgpu.Buffer
	group           *wgpu.BindGroup
	sampler         *wgpu.Sampler
	pages           map[*uipaint.Texture]uiPage
	capacity        int
	submission      uipaint.Submission
	passDescriptor  wgpu.RenderPassDescriptor
	revision        uint64
	width, height   float32
}

func (driver *nativeUIHost) prepareUI() error {
	if driver.ui == nil {
		return nil
	}

	shader, err := driver.device.CreateShaderModuleWGSL(uipaint.Shader)
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
		Label:  "UI MSDF and panels",
		Vertex: wgpu.VertexState{Module: shader, EntryPoint: "vertexMain", Buffers: uiVertexLayout()},
		Primitive: wgpu.PrimitiveState{
			Topology:  gputypes.PrimitiveTopologyTriangleList,
			FrontFace: gputypes.FrontFaceCCW,
			CullMode:  gputypes.CullModeNone,
		},
		Multisample: wgpu.MultisampleState{Count: 1, Mask: allSampleMask},
		Fragment: &wgpu.FragmentState{
			Module:     shader,
			EntryPoint: "fragmentMain",
			Targets: []wgpu.ColorTargetState{
				{Format: driver.format, WriteMask: gputypes.ColorWriteMaskAll, Blend: &wgpu.BlendState{Color: blend, Alpha: blend}},
			},
		},
	})
	if err != nil {
		return err
	}

	gpu := &uiGPU{pipeline: pipeline, pages: make(map[*uipaint.Texture]uiPage)}
	gpu.passDescriptor.ColorAttachments = []wgpu.RenderPassColorAttachment{
		{LoadOp: gputypes.LoadOpLoad, StoreOp: gputypes.StoreOpStore},
	}
	driver.uiGPU = gpu

	gpu.uniform, err = driver.device.CreateBuffer(
		&wgpu.BufferDescriptor{Size: 16, Usage: wgpu.BufferUsageUniform | wgpu.BufferUsageCopyDst},
	)
	if err != nil {
		return err
	}

	gpu.sampler, err = driver.device.CreateSampler(
		&wgpu.SamplerDescriptor{
			AddressModeU: gputypes.AddressModeClampToEdge,
			AddressModeV: gputypes.AddressModeClampToEdge,
			AddressModeW: gputypes.AddressModeClampToEdge,
			MinFilter:    gputypes.FilterModeLinear,
			MagFilter:    gputypes.FilterModeLinear,
		},
	)

	return err
}
func (driver *nativeUIHost) updateUI(frame *uipaint.Frame) error {
	if frame == nil {
		return nil
	}

	gpu := driver.uiGPU
	if gpu == nil {
		return errEmptySurfaceFrame
	}

	if gpu.revision != frame.Revision {
		if err := driver.validateUIPages(frame); err != nil {
			return err
		}
	}

	frame = gpu.submission.Update(frame)

	if gpu.width != frame.Width || gpu.height != frame.Height {
		linear := driver.format == gputypes.TextureFormatBGRA8UnormSrgb || driver.format == gputypes.TextureFormatRGBA8UnormSrgb
		if err := driver.queue.WriteBuffer(gpu.uniform, 0, uipaint.Viewport(frame.Width, frame.Height, linear)); err != nil {
			return err
		}

		gpu.width, gpu.height = frame.Width, frame.Height
	}

	if gpu.revision == frame.Revision {
		return nil
	}

	if len(frame.Data) == 0 {
		if err := driver.updateUIPages(frame); err != nil {
			return err
		}

		gpu.revision = frame.Revision

		return nil
	}

	full := gpu.revision != frame.Base
	if len(frame.Data) > gpu.capacity {
		if err := driver.growUIBuffer(len(frame.Data)); err != nil {
			return err
		}

		full = true
	}

	if full {
		if err := driver.queue.WriteBuffer(gpu.buffer, 0, frame.Data); err != nil {
			return err
		}
	} else {
		for _, r := range frame.Changes {
			if err := driver.queue.WriteBuffer(gpu.buffer, uint64(r.Offset), frame.Data[r.Offset:r.Offset+r.Size]); err != nil {
				return err
			}
		}
	}

	if err := driver.updateUIPages(frame); err != nil {
		return err
	}

	gpu.revision = frame.Revision

	return nil
}
func (driver *nativeUIHost) uploadUIPage(page *uipaint.Texture) (uiPage, error) {
	return driver.uploadPreparedPage(page, driver.uiGPU.pipeline, driver.uiGPU.sampler)
}

func (driver *nativeUIHost) validateUIPages(frame *uipaint.Frame) error {
	for _, batch := range frame.Batches {
		if batch.Count == 0 {
			continue
		}

		if _, exists := driver.uiGPU.pages[batch.Texture]; exists {
			continue
		}

		if _, err := driver.preparedPageFormat(batch.Texture); err != nil {
			return err
		}
	}

	return nil
}
func (driver *nativeUIHost) drawUI(encoder *wgpu.CommandEncoder, view *wgpu.TextureView, frame *uipaint.Frame) error {
	if frame == nil {
		return nil
	}

	gpu := driver.uiGPU

	frame = gpu.submission.Current()
	if len(frame.Data) == 0 {
		return nil
	}

	gpu.passDescriptor.ColorAttachments[0].View = view

	pass, err := encoder.BeginRenderPass(&gpu.passDescriptor)
	if err != nil {
		return err
	}
	defer pass.Release()
	defer pass.End()

	pass.SetPipeline(gpu.pipeline)
	pass.SetVertexBuffer(0, gpu.buffer, 0, uint64(gpu.capacity))
	pass.SetBindGroup(0, gpu.group, nil)

	for _, batch := range frame.Batches {
		if batch.Count == 0 {
			continue
		}

		pass.SetBindGroup(1, gpu.pages[batch.Texture].group, nil)
		pass.Draw(6, uint32(batch.Count), 0, uint32(batch.First))
		driver.metrics.DrawUI(6, uint64(batch.Count))
	}

	return nil
}

func uiVertexLayout() []wgpu.VertexBufferLayout {
	attributes := []wgpu.VertexAttribute{
		{Format: gputypes.VertexFormatFloat32x4, Offset: 0, ShaderLocation: 0},
		{Format: gputypes.VertexFormatFloat32x4, Offset: 64, ShaderLocation: 1},
	}

	return []wgpu.VertexBufferLayout{
		{ArrayStride: uipaint.Stride, StepMode: gputypes.VertexStepModeInstance, AttributeCount: 2, Attributes: &attributes[0]},
	}
}
func (driver *nativeUIHost) closeUI() {
	gpu := driver.uiGPU
	if gpu == nil {
		return
	}

	for _, page := range gpu.pages {
		page.group.Release()
		page.view.Release()
		page.texture.Release()
	}

	if gpu.group != nil {
		gpu.group.Release()
	}

	if gpu.buffer != nil {
		gpu.buffer.Release()
	}

	if gpu.uniform != nil {
		gpu.uniform.Release()
	}

	if gpu.sampler != nil {
		gpu.sampler.Release()
	}

	if gpu.pipeline != nil {
		gpu.pipeline.Release()
	}

	driver.uiGPU = nil
}

func (driver *nativeUIHost) growUIBuffer(size int) error {
	gpu := driver.uiGPU

	if gpu.group != nil {
		gpu.group.Release()
	}

	if gpu.buffer != nil {
		gpu.buffer.Release()
	}

	gpu.capacity = 256
	for gpu.capacity < size {
		gpu.capacity *= 2
	}

	var err error

	gpu.buffer, err = driver.device.CreateBuffer(
		&wgpu.BufferDescriptor{
			Size:  uint64(gpu.capacity),
			Usage: wgpu.BufferUsageStorage | wgpu.BufferUsageCopyDst | wgpu.BufferUsageVertex,
		},
	)
	if err != nil {
		return err
	}

	layout := gpu.pipeline.GetBindGroupLayout(0)
	gpu.group, err = driver.device.CreateBindGroup(
		&wgpu.BindGroupDescriptor{
			Layout: layout,
			Entries: []wgpu.BindGroupEntry{
				{Binding: 0, Buffer: gpu.uniform, Size: 16},
				{Binding: 1, Buffer: gpu.buffer, Size: uint64(gpu.capacity)},
			},
		},
	)
	layout.Release()

	if err != nil {
		return err
	}

	return nil
}

func (driver *nativeUIHost) updateUIPages(frame *uipaint.Frame) error {
	gpu := driver.uiGPU

	for _, batch := range frame.Batches {
		if batch.Count == 0 {
			continue
		}

		if _, ok := gpu.pages[batch.Texture]; !ok {
			page, err := driver.uploadUIPage(batch.Texture)
			if err != nil {
				return err
			}

			gpu.pages[batch.Texture] = page
		}
	}

	for _, batch := range frame.Batches {
		if batch.Count > 0 {
			page := gpu.pages[batch.Texture]
			page.used = frame.Revision
			gpu.pages[batch.Texture] = page
		}
	}

	for source, page := range gpu.pages {
		if page.used != frame.Revision {
			page.group.Release()
			page.view.Release()
			page.texture.Release()
			delete(gpu.pages, source)
		}
	}

	return nil
}
