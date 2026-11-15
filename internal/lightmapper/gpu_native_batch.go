//go:build !js

//nolint:mnd // Buffer layouts, flag words and dispatch widths mirror the production WGSL ABI.
package lightmapper

import (
	"bytes"
	"context"
	"encoding/binary"
	"math"
	"unsafe"

	"github.com/gogpu/gputypes"
	wgpu "github.com/karty-game/karty-webgpu/internal/webgpu"
)

type nativeBatch struct {
	mapped                                           bool
	uniform, tile, paths, indirect, output, readback *wgpu.Buffer
	groups                                           [3]*wgpu.BindGroup
	sizes                                            nativeBatchSizes
}

func (batch *nativeBatch) close() {
	for _, group := range batch.groups {
		if group != nil {
			group.Release()
		}
	}

	if batch.mapped {
		_ = batch.readback.Unmap()
	}

	for _, buffer := range []*wgpu.Buffer{batch.uniform, batch.tile, batch.paths, batch.indirect, batch.output, batch.readback} {
		releaseNativeBuffer(buffer)
	}
}
func (executor *nativeGPUExecutor) Run(ctx context.Context, scene GPUScene, tile GPUTile) ([]byte, error) {
	if ctx == nil {
		return nil, ErrInvalid
	}

	executor.mu.Lock()
	defer executor.mu.Unlock()

	if executor.closed.Load() {
		return nil, errGPUExecution
	}

	life, cancel := context.WithCancel(ctx)
	//nolint:contextcheck // Merge the independent caller and device lifetime contexts.
	stop := context.AfterFunc(executor.ctx, cancel)
	defer stop()
	defer cancel()

	if err := life.Err(); err != nil {
		return nil, err
	}

	sizes, err := nativeBatchAdmission(scene, tile, executor.memoryBytes, executor.limits, executor.traceRays)
	if err != nil {
		return nil, err
	}

	upload := executor.residentKey != scene.Key
	if upload && !gpuValidateWideScene(scene.Data) {
		return nil, ErrInvalid
	}

	if !upload && executor.residentBytes != uint64(len(scene.Data)) {
		return nil, ErrInvalid
	}

	batch := nativeBatch{sizes: sizes}
	defer batch.close()

	var result []byte

	err = executor.checked(func() error {
		if err := executor.prepareBatch(scene, tile, &batch, upload); err != nil {
			return err
		}

		if err := executor.execute(life, tile, &batch); err != nil {
			return err
		}

		var err error

		result, err = executor.readBatchResult(life, tile, &batch)

		return err
	})
	if err != nil {
		return nil, err
	}

	if err = life.Err(); err != nil {
		return nil, err
	}

	return result, nil
}
func (executor *nativeGPUExecutor) prepareBatch(scene GPUScene, tile GPUTile, batch *nativeBatch, upload bool) error {
	var err error

	if upload {
		if executor.uploads.Load() == math.MaxUint64 {
			return ErrBounds
		}

		releaseNativeBuffer(executor.scene)
		executor.scene = nil

		executor.scene, err = executor.gpu.CreateBuffer(&wgpu.BufferDescriptor{Label: "resident bake scene", Size: uint64(len(scene.Data)),
			Usage: gputypes.BufferUsageStorage | gputypes.BufferUsageCopyDst})
		if err != nil {
			return err
		}

		if err = executor.queue.WriteBuffer(executor.scene, 0, scene.Data); err != nil {
			return err
		}

		executor.residentKey, executor.residentBytes = scene.Key, uint64(len(scene.Data))
		executor.uploads.Add(1)
	}

	if err = executor.createBatchBuffers(tile, batch); err != nil {
		return err
	}

	return executor.createBatchGroups(batch)
}
func (executor *nativeGPUExecutor) createBatchBuffers(tile GPUTile, batch *nativeBatch) error {
	pathUsage := gputypes.BufferUsageStorage
	if executor.traceRays || tile.Mode == "" {
		pathUsage |= gputypes.BufferUsageCopySrc
	}

	buffers := []struct {
		target **wgpu.Buffer
		name   string
		size   uint64
		usage  gputypes.BufferUsage
	}{
		{&batch.uniform, "bake parameters", 64, gputypes.BufferUsageUniform | gputypes.BufferUsageCopyDst},
		{&batch.tile, "bake receivers", uint64(len(tile.Data)), gputypes.BufferUsageStorage | gputypes.BufferUsageCopyDst},
		{&batch.paths, "bake paths", batch.sizes.paths, pathUsage},
		{&batch.indirect, "bake indirect dispatch", 12, gputypes.BufferUsageStorage | gputypes.BufferUsageIndirect},
		{&batch.output, "bake estimates", batch.sizes.output, gputypes.BufferUsageStorage | gputypes.BufferUsageCopySrc},
		{&batch.readback, "bake readback", batch.sizes.readback, gputypes.BufferUsageMapRead | gputypes.BufferUsageCopyDst},
	}
	for _, spec := range buffers {
		buffer, err := executor.gpu.CreateBuffer(&wgpu.BufferDescriptor{Label: spec.name, Size: spec.size, Usage: spec.usage})
		if err != nil {
			return err
		}

		*spec.target = buffer
	}

	return executor.queue.WriteBuffer(batch.tile, 0, tile.Data)
}
func nativeGroupEntry(binding uint32, buffer *wgpu.Buffer) wgpu.BindGroupEntry {
	return wgpu.BindGroupEntry{Binding: binding, Buffer: buffer, Size: buffer.Size()}
}
func (executor *nativeGPUExecutor) createBatchGroups(batch *nativeBatch) error {
	entries := []wgpu.BindGroupEntry{
		nativeGroupEntry(0, batch.uniform),
		nativeGroupEntry(1, executor.scene),
		nativeGroupEntry(2, batch.tile),
		nativeGroupEntry(3, batch.paths),
		nativeGroupEntry(4, batch.output),
	}

	groups := [][]wgpu.BindGroupEntry{entries, {entries[0], entries[3], nativeGroupEntry(5, batch.indirect)},
		{entries[0], entries[1], entries[2], entries[3], nativeGroupEntry(6, executor.scene)}}
	for index, bindings := range groups {
		var err error

		batch.groups[index], err = executor.gpu.CreateBindGroup(
			&wgpu.BindGroupDescriptor{Layout: executor.layouts[index], Entries: bindings},
		)
		if err != nil {
			return err
		}
	}

	return nil
}
func (executor *nativeGPUExecutor) recordTraceCounts(data []byte, count uint64) error {
	plane := count * gpuWordBytes
	if uint64(len(data)) != plane*4+gpuWordBytes || binary.LittleEndian.Uint32(data[plane*4:]) != 0 {
		return ErrBounds
	}

	totals := executor.traceCounts

	for owner := range count {
		rays := uint64(binary.LittleEndian.Uint32(data[owner*4:])) | uint64(binary.LittleEndian.Uint32(data[plane+owner*4:]))<<32

		shadows := uint64(
			binary.LittleEndian.Uint32(data[plane*2+owner*4:]),
		) | uint64(
			binary.LittleEndian.Uint32(data[plane*3+owner*4:]),
		)<<32
		if shadows > rays || rays > math.MaxUint64-totals.Rays || shadows > math.MaxUint64-totals.ShadowRays {
			return ErrBounds
		}

		totals.Rays += rays
		totals.ShadowRays += shadows
	}

	executor.traceCounts = totals

	return nil
}

func (executor *nativeGPUExecutor) readBatchResult(ctx context.Context, tile GPUTile, batch *nativeBatch) ([]byte, error) {
	if err := executor.mapReadback(ctx, batch.readback, batch.sizes.readback); err != nil {
		return nil, err
	}

	batch.mapped = true

	mapped := batch.readback.GetMappedRange(0, batch.sizes.readback)
	if mapped == nil {
		return nil, ErrInvalid
	}
	// Mapping has completed and this serialized owner retains the buffer until
	// the result is copied. Avoid the binding's unimplemented native MapState query.
	//nolint:gosec // G103: admitted readback bytes stay mapped and owned until the private copy completes.
	data := unsafe.Slice((*byte)(mapped), batch.sizes.readback)
	if uint64(len(data)) != batch.sizes.readback {
		return nil, ErrInvalid
	}

	if executor.traceRays && tile.Mode != gpuModeReconstruct {
		if err := executor.recordTraceCounts(data[batch.sizes.output:], uint64(tile.Count)); err != nil {
			return nil, err
		}
	}

	return bytes.Clone(data[:batch.sizes.output]), nil
}
