//go:build js && wasm

package browser

import (
	"fmt"
	"syscall/js"

	"github.com/karty-game/ashura/internal/platform"
	"github.com/karty-game/ashura/internal/render"
	"github.com/karty-game/ashura/internal/texturedata"
)

type gpuDecals struct {
	uploads          render.DecalUploads
	authored         gpuStaticDecals
	shader           func(string, bool) string
	uniform, staging js.Value
	atlas            *texturedata.Texture
	groups           [4]js.Value
	groupIndex       int
}

func RegisterDecals(host any) error {
	driver, err := materialFeatureDriver(host)
	if err != nil {
		return err
	}

	if driver.decals != nil {
		return fmt.Errorf("%s: %w", platform.DecalFeature, platform.ErrFeatureDuplicate)
	}

	buffer := driver.device.Call("createBuffer", jsObject(map[string]any{
		labelProperty: "decal projectors", sizeProperty: render.DecalUniformSize, usageProperty: uniformBufferUsage,
	}))
	if buffer.IsNull() || buffer.IsUndefined() {
		return errDrawBuffer
	}

	group := 2
	if driver.clusteredLights != nil {
		group++
	}

	driver.decals = &gpuDecals{
		uniform: buffer, staging: js.Global().Get("Uint8Array").New(render.DecalRecordBytes),
		groupIndex: group, shader: render.DecalMaterialShader,
	}

	return driver.decals.authored.prepare(driver.device)
}

func (driver *Driver) prepareDecals(frame *render.DecalFrame) error {
	decals := driver.decals
	if decals == nil {
		if frame != nil {
			return platform.ErrFeatureMissing
		}

		return nil
	}

	if err := decals.uploads.Update(frame, func(offset int, data []byte) error {
		js.CopyBytesToJS(decals.staging, data)
		driver.queue.Call("writeBuffer", decals.uniform, offset, decals.staging, 0, len(data))

		return nil
	}); err != nil {
		return err
	}

	driver.initMaterials()

	atlas := decals.atlas
	if atlas == nil {
		atlas = driver.white
	}

	if frame != nil {
		atlas = frame.Atlas
	}

	if atlas == decals.atlas {
		return nil
	}

	view, err := driver.textureView(atlas)
	if err != nil {
		return fmt.Errorf("prepare decal atlas: %w", err)
	}

	var groups [4]js.Value

	entries := js.ValueOf([]any{
		clusteredBufferEntry(0, decals.uniform),
		jsObject(map[string]any{bindingProperty: 1, resourceProperty: view}),
		clusteredBufferEntry(2, decals.authored.view),
		clusteredBufferEntry(3, decals.authored.records),
		clusteredBufferEntry(4, decals.authored.tiles),
		clusteredBufferEntry(5, decals.authored.masks),
	})

	for variant := range groups {
		pipeline := driver.materialPipeline(variant >= 2, variant%2 == 1)
		if pipeline.IsUndefined() || pipeline.IsNull() {
			continue
		}

		groups[variant] = driver.device.Call("createBindGroup", jsObject(map[string]any{
			layoutProperty: pipeline.Call("getBindGroupLayout", decals.groupIndex), entriesProperty: entries,
		}))
	}

	decals.atlas, decals.groups = atlas, groups

	return nil
}

func (driver *Driver) bindDecals(pass js.Value, draw render.DrawCall) {
	if driver.decals == nil || draw.Impostor != nil {
		return
	}

	pass.Call("setBindGroup", driver.decals.groupIndex, driver.decals.groups[draw.DecalVariant()])
}

func (driver *Driver) clearDecalAtlas(atlas *texturedata.Texture) {
	if driver.decals != nil && driver.decals.atlas == atlas {
		driver.decals.groups = [4]js.Value{}
		driver.decals.atlas = nil
	}
}

func (driver *Driver) closeDecals() {
	if driver.decals != nil {
		driver.decals.authored.release()
		driver.decals.uniform.Call("destroy")
		driver.decals = nil
	}
}
