//go:build linux

package native

import (
	"fmt"

	"github.com/gogpu/gputypes"
	"github.com/karty-game/ashura/internal/render"
	wgpu "github.com/karty-game/ashura/internal/webgpu"
)

type gpuStaticDecals struct {
	uploads                     render.StaticDecalUploads
	view, records, masks, tiles *wgpu.Buffer
	pipelines                   [3]*wgpu.ComputePipeline
	groups                      [3]*wgpu.BindGroup
}

//nolint:mnd // Binding numbers identify the decal compute shader ABI.
func (decals *gpuStaticDecals) prepare(device *wgpu.Device) error {
	if err := decals.prepareBuffers(device); err != nil {
		return err
	}

	module, err := device.CreateShaderModuleWGSL(render.DecalTileShader())
	if err != nil {
		return err
	}
	defer module.Release()

	view := wgpu.BindGroupEntry{Binding: 0, Buffer: decals.view, Size: render.DecalViewBytes}
	authored := wgpu.BindGroupEntry{Binding: 1, Buffer: decals.records, Size: render.StaticDecalBytes}
	mask := wgpu.BindGroupEntry{Binding: 2, Buffer: decals.masks, Size: render.DecalTileStorageBytes}
	tiles := wgpu.BindGroupEntry{Binding: 3, Buffer: decals.tiles, Size: render.DecalTileBytes}
	stages := [...]struct {
		name    string
		entries []wgpu.BindGroupEntry
	}{
		{"clearMasks", []wgpu.BindGroupEntry{mask, tiles}},
		{"binProjectors", []wgpu.BindGroupEntry{view, authored, mask, tiles}},
		{"compactTiles", []wgpu.BindGroupEntry{mask, tiles}},
	}

	for index, stage := range stages {
		pipeline, err := device.CreateComputePipeline(&wgpu.ComputePipelineDescriptor{
			Label: stage.name, Module: module, EntryPoint: stage.name,
		})
		if err != nil {
			return err
		}

		decals.pipelines[index] = pipeline
		layout := pipeline.GetBindGroupLayout(0)
		decals.groups[index], err = device.CreateBindGroup(&wgpu.BindGroupDescriptor{Layout: layout, Entries: stage.entries})
		layout.Release()

		if err != nil {
			return err
		}
	}

	return nil
}

func (decals *gpuStaticDecals) prepareBuffers(device *wgpu.Device) error {
	const (
		storage         = gputypes.BufferUsageStorage
		copyDestination = gputypes.BufferUsageCopyDst
	)

	for _, entry := range [...]struct {
		output **wgpu.Buffer
		label  string
		size   uint64
		usage  gputypes.BufferUsage
	}{
		{&decals.view, "decal view", render.DecalViewBytes, gputypes.BufferUsageUniform | copyDestination},
		{&decals.records, "authored decal records", render.StaticDecalBytes, storage | copyDestination},
		{&decals.masks, "decal tile masks", render.DecalTileStorageBytes, storage | copyDestination},
		{&decals.tiles, "decal tile indices", render.DecalTileBytes, storage},
	} {
		buffer, err := device.CreateBuffer(&wgpu.BufferDescriptor{Label: entry.label, Size: entry.size, Usage: entry.usage})
		if err != nil {
			return err
		}

		*entry.output = buffer
	}

	return nil
}

func (driver *Driver) recordStaticDecals(encoder *wgpu.CommandEncoder, frame render.Frame) error {
	if driver.decals == nil || len(frame.Draws) == 0 {
		return nil
	}

	decals := &driver.decals.authored

	var authored *render.StaticDecalFrame
	if frame.Decals != nil {
		authored = frame.Decals.Static
	}

	dirty, err := decals.uploads.Update(authored, func(offset int, data []byte) error {
		//nolint:gosec // Static pages are indexed within the allocated storage buffer.
		return driver.queue.WriteBuffer(decals.records, uint64(offset), data)
	})
	if err != nil {
		return err
	}

	orderChanged, err := decals.uploads.Order(authored, func(data []byte) error {
		return driver.queue.WriteBuffer(decals.masks, render.DecalTileMaskBytes, data)
	})
	if err != nil {
		return err
	}

	viewChanged, err := decals.uploads.View(frame, int(driver.width), int(driver.height), func(data []byte) error {
		return driver.queue.WriteBuffer(decals.view, 0, data)
	})
	if err != nil {
		return err
	}

	if authored == nil || authored.End == 0 || !dirty && !orderChanged && !viewChanged {
		return nil
	}

	//nolint:gosec // End is a bounded page-aligned record count.
	return decals.run(encoder, uint32(authored.End))
}

func (decals *gpuStaticDecals) run(encoder *wgpu.CommandEncoder, end uint32) error {
	const lanes = 64

	dispatches := [3]uint32{render.DecalTileMaskBytes / 4 / lanes, (end + lanes - 1) / lanes, render.DecalTileCount}

	pass, err := encoder.BeginComputePass(&wgpu.ComputePassDescriptor{Label: "bin authored decals"})
	if err != nil {
		return fmt.Errorf("bin authored decals: %w", err)
	}
	defer pass.Release()

	for index, count := range dispatches {
		pass.SetPipeline(decals.pipelines[index])
		pass.SetBindGroup(0, decals.groups[index], nil)
		pass.DispatchWorkgroups(count, 1, 1)
	}

	pass.End()

	return nil
}

func (decals *gpuStaticDecals) release() {
	for index, group := range decals.groups {
		if group != nil {
			group.Release()
		}

		if pipeline := decals.pipelines[index]; pipeline != nil {
			pipeline.Release()
		}
	}

	for _, buffer := range []*wgpu.Buffer{decals.view, decals.records, decals.masks, decals.tiles} {
		if buffer != nil {
			buffer.Release()
		}
	}
}
