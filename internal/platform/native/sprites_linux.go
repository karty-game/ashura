//go:build linux

package native

import (
	"fmt"

	"github.com/gogpu/gputypes"
	"github.com/karty-game/ashura/internal/spritepaint"
	wgpu "github.com/karty-game/ashura/internal/webgpu"
)

type spriteGPU struct {
	pipeline        *wgpu.RenderPipeline
	uniform, buffer *wgpu.Buffer
	group           *wgpu.BindGroup
	sampler         *wgpu.Sampler
	pages           map[*spritepaint.Texture]uiPage
	batches         []spritepaint.Batch
	passColors      [1]wgpu.RenderPassColorAttachment
	passDescriptor  wgpu.RenderPassDescriptor
	capacity        int
	revision        uint64
	batchesRevision uint64
	width, height   float32
}

func (driver *spriteFeature) updateSprites(frame *spritepaint.Frame) error {
	if frame == nil {
		if driver.sprites != nil {
			driver.sprites.batches = nil
		}

		return nil
	}

	if driver.sprites == nil {
		return errEmptySurfaceFrame
	}

	gpu := driver.sprites
	if gpu.batchesRevision != frame.BatchesRevision {
		if err := driver.validateSpritePages(frame); err != nil {
			return err
		}
	}

	if gpu.width != frame.Width || gpu.height != frame.Height {
		linear := driver.format == gputypes.TextureFormatBGRA8UnormSrgb || driver.format == gputypes.TextureFormatRGBA8UnormSrgb
		if err := driver.queue.WriteBuffer(gpu.uniform, 0, spritepaint.Viewport(frame.Width, frame.Height, linear)); err != nil {
			return fmt.Errorf("update sprite viewport: %w", err)
		}

		gpu.width, gpu.height = frame.Width, frame.Height
	}

	if gpu.revision == frame.Revision {
		gpu.batches = frame.Batches

		return nil
	}

	if err := driver.writeSpriteRecords(frame); err != nil {
		return err
	}

	if gpu.batchesRevision != frame.BatchesRevision {
		if err := driver.updateSpritePages(frame); err != nil {
			return err
		}

		gpu.batchesRevision = frame.BatchesRevision
	}

	gpu.batches, gpu.revision = frame.Batches, frame.Revision

	return nil
}

func (driver *spriteFeature) writeSpriteRecords(frame *spritepaint.Frame) error {
	if len(frame.Data) == 0 {
		return nil
	}

	gpu := driver.sprites

	full := gpu.revision != frame.Base
	if len(frame.Data) > gpu.capacity {
		if err := driver.growSpriteBuffer(len(frame.Data)); err != nil {
			return err
		}

		full = true
	}

	if full {
		return driver.queue.WriteBuffer(gpu.buffer, 0, frame.Data)
	}

	for _, change := range frame.Changes {
		offset := uint64(change.Offset) //nolint:gosec // Dirty offsets reference admitted sprite records.
		if err := driver.queue.WriteBuffer(
			gpu.buffer,
			offset,
			frame.Data[change.Offset:change.Offset+change.Size],
		); err != nil {
			return err
		}
	}

	return nil
}

func (driver *spriteFeature) growSpriteBuffer(size int) error {
	gpu := driver.sprites

	const minimumCapacity = 256

	capacity := minimumCapacity
	for capacity < size {
		capacity *= 2
	}

	limits := driver.device.Limits()

	bufferSize := uint64(capacity)
	if bufferSize > limits.MaxBufferSize || bufferSize > limits.MaxStorageBufferBindingSize {
		return fmt.Errorf("sprite records: %w", errComputeLimit)
	}

	buffer, err := driver.device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "retained Screen2D records", Size: bufferSize,
		Usage: wgpu.BufferUsageStorage | wgpu.BufferUsageCopyDst | wgpu.BufferUsageVertex,
	})
	if err != nil {
		return err
	}

	layout := gpu.pipeline.GetBindGroupLayout(0)
	defer layout.Release()

	group, err := driver.device.CreateBindGroup(&wgpu.BindGroupDescriptor{
		Layout: layout,
		Entries: []wgpu.BindGroupEntry{
			{Binding: 0, Buffer: gpu.uniform, Size: spriteViewportSize},
			{Binding: 1, Buffer: buffer, Size: bufferSize},
		},
	})
	if err != nil {
		buffer.Release()

		return err
	}

	if gpu.group != nil {
		gpu.group.Release()
		gpu.buffer.Release()
	}

	gpu.buffer, gpu.group, gpu.capacity = buffer, group, capacity

	return nil
}

func (driver *spriteFeature) drawSprites(encoder *wgpu.CommandEncoder, view *wgpu.TextureView) error {
	gpu := driver.sprites
	if gpu == nil || len(gpu.batches) == 0 {
		return nil
	}

	gpu.passColors[0].View = view

	pass, err := encoder.BeginRenderPass(&gpu.passDescriptor)
	if err != nil {
		return err
	}
	defer pass.Release()
	defer pass.End()

	pass.SetPipeline(gpu.pipeline)

	bufferBytes := uint64(gpu.capacity) //nolint:gosec // Capacity is positive and bounded by admitted bytes.
	pass.SetVertexBuffer(
		0,
		gpu.buffer,
		0,
		bufferBytes,
	)
	pass.SetBindGroup(0, gpu.group, nil)

	for _, batch := range gpu.batches {
		if batch.Count == 0 || batch.Clip[2] == 0 || batch.Clip[3] == 0 {
			continue
		}

		pass.SetScissorRect(batch.Clip[0], batch.Clip[1], batch.Clip[2], batch.Clip[3])
		pass.SetBindGroup(1, gpu.pages[batch.Texture].group, nil)

		count := uint32(batch.Count) //nolint:gosec // Batch counts are bounded admitted sprite counts.
		first := uint32(batch.First) //nolint:gosec // Batch offsets address bounded admitted sprite slots.
		pass.Draw(
			spriteVertices,
			count,
			0,
			first,
		)
		driver.metrics.DrawSprites(
			spriteVertices,
			uint64(count),
		)
	}

	return nil
}

func (driver *spriteFeature) closeSprites() {
	gpu := driver.sprites
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

	driver.sprites = nil
}
