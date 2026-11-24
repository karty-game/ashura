//go:build js && wasm

package browser

import (
	"fmt"
	"syscall/js"

	"github.com/karty-game/ashura/internal/render"
)

type gpuStaticDecals struct {
	uploads                     render.StaticDecalUploads
	view, records, masks, tiles js.Value
	viewBytes, pageBytes        js.Value
	orderBytes                  js.Value
	pipelines, groups           [3]js.Value
}

func (decals *gpuStaticDecals) prepare(device js.Value) error {
	const (
		storage         = 0x80
		copyDestination = 0x08
	)

	makeBuffer := func(label string, size, usage int) js.Value {
		return device.Call("createBuffer", jsObject(map[string]any{labelProperty: label, sizeProperty: size, usageProperty: usage}))
	}
	decals.view = makeBuffer("decal view", render.DecalViewBytes, uniformBufferUsage)
	decals.records = makeBuffer("authored decal records", render.StaticDecalBytes, storage|copyDestination)
	decals.masks = makeBuffer("decal tile masks", render.DecalTileStorageBytes, storage|copyDestination)
	decals.tiles = makeBuffer("decal tile indices", render.DecalTileBytes, storage)
	decals.viewBytes = js.Global().Get("Uint8Array").New(render.DecalViewBytes)
	decals.pageBytes = js.Global().Get("Uint8Array").New(render.StaticDecalPageBytes)
	decals.orderBytes = js.Global().Get("Uint8Array").New(render.StaticDecalOrderBytes)

	module := device.Call("createShaderModule", jsObject(map[string]any{
		labelProperty: "authored decal culling", shaderCodeProperty: render.DecalTileShader(),
	}))
	view := clusteredBufferEntry(0, decals.view)
	authored := clusteredBufferEntry(1, decals.records)
	mask := clusteredBufferEntry(2, decals.masks)
	tiles := clusteredBufferEntry(3, decals.tiles)
	stages := [...]struct {
		name    string
		entries []any
	}{
		{"clearMasks", []any{mask, tiles}},
		{"binProjectors", []any{view, authored, mask, tiles}},
		{"compactTiles", []any{mask, tiles}},
	}

	for index, stage := range stages {
		pipeline, err := awaitPromise(device.Call("createComputePipelineAsync", jsObject(map[string]any{
			labelProperty: stage.name, layoutProperty: automaticLayout, particleComputeProperty: jsObject(map[string]any{
				moduleProperty: module, entryPointProperty: stage.name,
			}),
		})))
		if err != nil {
			return fmt.Errorf("initialize authored decal culling: %w", err)
		}

		decals.pipelines[index] = pipeline
		decals.groups[index] = device.Call("createBindGroup", jsObject(map[string]any{
			layoutProperty: pipeline.Call("getBindGroupLayout", 0), entriesProperty: js.ValueOf(stage.entries),
		}))
	}

	return nil
}

func (driver *Driver) recordStaticDecals(encoder js.Value, frame render.Frame) error {
	if driver.decals == nil || len(frame.Draws) == 0 {
		return nil
	}

	decals := &driver.decals.authored

	var authored *render.StaticDecalFrame
	if frame.Decals != nil {
		authored = frame.Decals.Static
	}

	dirty, err := decals.uploads.Update(authored, func(offset int, data []byte) error {
		js.CopyBytesToJS(decals.pageBytes, data)
		driver.queue.Call("writeBuffer", decals.records, offset, decals.pageBytes, 0, len(data))

		return nil
	})
	if err != nil {
		return err
	}

	orderChanged, err := decals.uploads.Order(authored, func(data []byte) error {
		js.CopyBytesToJS(decals.orderBytes, data)
		driver.queue.Call("writeBuffer", decals.masks, render.DecalTileMaskBytes, decals.orderBytes)

		return nil
	})
	if err != nil {
		return err
	}

	viewChanged, err := decals.uploads.View(frame, driver.canvasWidth, driver.canvasHeight, func(data []byte) error {
		js.CopyBytesToJS(decals.viewBytes, data)
		driver.queue.Call("writeBuffer", decals.view, 0, decals.viewBytes)

		return nil
	})
	if err != nil {
		return err
	}

	if authored == nil || authored.End == 0 || !dirty && !orderChanged && !viewChanged {
		return nil
	}

	const lanes = 64

	dispatches := [3]int{render.DecalTileMaskBytes / 4 / lanes, (authored.End + lanes - 1) / lanes, render.DecalTileCount}

	encoder.Call("beginComputePass")

	pass := driver.computePass
	for index, count := range dispatches {
		pass.Call("setPipeline", decals.pipelines[index])
		pass.Call("setBindGroup", 0, decals.groups[index])
		pass.Call("dispatchWorkgroups", count, 1, 1)
	}

	pass.Call("end")

	return nil
}

func (decals *gpuStaticDecals) release() {
	for _, buffer := range []js.Value{decals.view, decals.records, decals.masks, decals.tiles} {
		if !buffer.IsUndefined() && !buffer.IsNull() {
			buffer.Call("destroy")
		}
	}
}
