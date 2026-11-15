//go:build js && wasm

package browser

import (
	"syscall/js"

	"github.com/karty-game/karty-webgpu/internal/impostordata"
	"github.com/karty-game/karty-webgpu/internal/render"
	"github.com/karty-game/karty-webgpu/internal/texturedata"
)

const (
	sampledTextureUsage = 0x06
	uniformBufferUsage  = 0x48
)

type gpuTexture struct {
	image, view js.Value
	bytes       uint64
}
type gpuMaterial struct {
	config                *impostordata.Document
	impostor              *gpuImpostor
	culled                bool
	state                 render.UniformState
	occlusion             *texturedata.Texture
	uniform, array, group js.Value
	albedo, normal        *texturedata.Texture
	reflection            *texturedata.Texture
	lightmap              *texturedata.Texture
	rnm                   [3]*texturedata.Texture
	seen                  bool
	source                bool
}

func (driver *Driver) materialGroup(draw render.DrawCall, frame render.Frame) (js.Value, error) {
	if err := driver.requireMaterialRenderer(draw); err != nil {
		return js.Undefined(), err
	}

	driver.initMaterials()

	albedo, normal, occlusion, reflection := draw.Albedo, draw.Normal, draw.AO, draw.Reflection
	if albedo == nil {
		albedo = driver.white
	}

	if normal == nil {
		normal = driver.flat
	}

	if draw.Source || draw.Impostor != nil {
		occlusion = nil
	} else if occlusion == nil {
		occlusion = driver.whiteAO
	}

	if reflection == nil {
		reflection = driver.white
	}

	lightmap := draw.Lightmap
	if lightmap == nil {
		lightmap = driver.whiteAO
	}

	entry, size := driver.materialEntry(draw)

	entry.seen = true
	if entry.culled != draw.CullBackfaces {
		entry.group = js.Undefined()
		entry.culled = draw.CullBackfaces
	}

	if entry.bindingsChanged(albedo, normal, occlusion, reflection, lightmap, draw.RNM) {
		if err := driver.bindMaterial(entry, albedo, normal, occlusion, reflection, lightmap, draw.RNM); err != nil {
			return js.Undefined(), err
		}
	}

	if entry.state.Update(draw, frame) {
		js.CopyBytesToJS(entry.array, entry.state.Data[:size])

		driver.queue.Call("writeBuffer", entry.uniform, 0, entry.array)
	}

	return entry.group, nil
}

func (driver *Driver) materialEntry(draw render.DrawCall) (*gpuMaterial, uint32) {
	entry := driver.materials[draw.ID]
	if entry != nil && (entry.source != draw.Source || entry.config != draw.Impostor) {
		entry.release()
		delete(driver.materials, draw.ID)

		entry = nil
	}

	size := render.UniformSize(draw.Source)
	if entry == nil {
		entry = &gpuMaterial{
			source: draw.Source,
			uniform: driver.device.Call(
				"createBuffer",
				jsObject(map[string]any{sizeProperty: size, usageProperty: uniformBufferUsage}),
			),
			array: js.Global().Get("Uint8Array").New(size),
		}
		if draw.Impostor != nil {
			entry.impostor = driver.acquireImpostor(draw.Impostor)
			entry.config = draw.Impostor
		}

		driver.materials[draw.ID] = entry
	}

	return entry, size
}

func (driver *Driver) initMaterials() {
	if driver.materials == nil {
		if driver.textures == nil {
			driver.textures = make(map[*texturedata.Texture]gpuTexture)
		}

		driver.materials = make(map[uint32]*gpuMaterial)
		driver.white = texturedata.Solid([4]byte{255, 255, 255, 255}, texturedata.SRGB)
		driver.flat = texturedata.Solid([4]byte{128, 128, 255, 255}, texturedata.Linear)
		driver.whiteAO = texturedata.Solid([4]byte{255, 255, 255, 255}, texturedata.Linear)
		driver.sampler = driver.device.Call(
			"createSampler",
			jsObject(
				map[string]any{
					addressModeUProperty: "repeat",
					addressModeVProperty: "repeat",
					magFilterProperty:    linearFilter,
					minFilterProperty:    linearFilter,
					mipmapFilterProperty: linearFilter,
					"maxAnisotropy":      render.MaterialAnisotropy,
				},
			),
		)
	}
}

func (driver *Driver) bindMaterial(
	entry *gpuMaterial,
	albedo, normal, occlusion, reflection, lightmap *texturedata.Texture,
	rnm [3]*texturedata.Texture,
) error {
	colorView, err := driver.textureView(albedo)
	if err != nil {
		return err
	}

	normalView, err := driver.textureView(normal)
	if err != nil {
		return err
	}

	entries := []any{
		jsObject(map[string]any{bindingProperty: 0, resourceProperty: jsObject(map[string]any{"buffer": entry.uniform})}),
		jsObject(map[string]any{bindingProperty: 1, resourceProperty: driver.sampler}),
		jsObject(map[string]any{bindingProperty: render.AlbedoBinding, resourceProperty: colorView}),
		jsObject(map[string]any{bindingProperty: render.NormalBinding, resourceProperty: normalView}),
	}
	switch {
	case entry.impostor != nil:
		entries[1] = jsObject(map[string]any{bindingProperty: 1, resourceProperty: driver.impostorSampler})
		entries = append(entries, jsObject(map[string]any{bindingProperty: render.OcclusionBinding,
			resourceProperty: jsObject(map[string]any{gpuBufferProperty: entry.impostor.buffer})}))
	case entry.source:
		entries, err = driver.rnmTextureBindings(entries, rnm)
		if err != nil {
			return err
		}
	default:
		entries, err = driver.standardTextureBindings(entries, occlusion, reflection, lightmap)
		if err != nil {
			return err
		}
	}

	pipeline := driver.materialPipeline(entry.source, entry.culled)
	if entry.config != nil {
		pipeline = driver.impostorPipeline
	}

	entry.group = driver.device.Call("createBindGroup", jsObject(map[string]any{
		layoutProperty:  pipeline.Call("getBindGroupLayout", 0),
		entriesProperty: js.ValueOf(entries),
	}))

	entry.albedo, entry.normal, entry.occlusion, entry.reflection = albedo, normal, occlusion, reflection

	entry.lightmap, entry.rnm = lightmap, rnm

	return nil
}

func (driver *Driver) textureView(source *texturedata.Texture) (js.Value, error) {
	if driver.textures == nil {
		driver.textures = make(map[*texturedata.Texture]gpuTexture)
	}

	if cached, ok := driver.textures[source]; ok {
		return cached.view, nil
	}

	width, height := source.Size()

	format, err := driver.preparedTextureFormat(source)
	if err != nil {
		return js.Undefined(), err
	}

	encoding := texturedata.Encoding(source)

	mipLevels := len(texturedata.Levels(source))
	usage := sampledTextureUsage

	if texturedata.IsStorage(source) {
		const storageSampledTextureUsage = 0x0c

		mipLevels = 1
		usage = storageSampledTextureUsage
	}

	image := driver.device.Call(
		"createTexture",
		jsObject(
			map[string]any{
				formatProperty: format, "mipLevelCount": mipLevels,
				usageProperty: usage,
				sizeProperty:  jsObject(map[string]any{widthProperty: width, heightProperty: height, depthOrArrayLayersProperty: 1}),
			},
		),
	)
	if image.IsUndefined() || image.IsNull() {
		return js.Undefined(), errTextureUpload
	}

	for level, pixels := range texturedata.Levels(source) {
		data := js.Global().Get("Uint8Array").New(len(pixels))
		js.CopyBytesToJS(data, pixels)

		levelWidth, levelHeight := max(width>>level, 1), max(height>>level, 1)

		rowBytes, rows := texturedata.Layout(encoding, levelWidth, levelHeight)
		levelWidth, levelHeight = texturedata.CopyExtent(encoding, levelWidth, levelHeight)

		driver.queue.
			Call(
				"writeTexture",
				jsObject(map[string]any{"texture": image, "mipLevel": level}),
				data,
				jsObject(map[string]any{"bytesPerRow": rowBytes, "rowsPerImage": rows}),
				jsObject(map[string]any{widthProperty: levelWidth, heightProperty: levelHeight, depthOrArrayLayersProperty: 1}),
			)
	}

	view := image.Call("createView")
	bytes := texturedata.ByteSize(source)
	driver.textures[source] = gpuTexture{image: image, view: view, bytes: bytes}
	driver.textureBytes += bytes

	return view, nil
}

// UnloadTexture releases the cached upload after scene references are detached.
func (driver *Driver) UnloadTexture(source *texturedata.Texture) {
	for id, entry := range driver.materials {
		if entry.albedo == source || entry.normal == source || entry.occlusion == source || entry.reflection == source ||
			entry.lightmap == source || entry.rnm[0] == source || entry.rnm[1] == source || entry.rnm[2] == source {
			entry.release()
			delete(driver.materials, id)
		}
	}

	if cached, ok := driver.textures[source]; ok {
		cached.image.Call("destroy")
		driver.textureBytes -= cached.bytes
		delete(driver.textures, source)
	}
}

func (driver *Driver) pruneMaterials() {
	for id, entry := range driver.materials {
		if !entry.seen {
			entry.release()
			delete(driver.materials, id)
		}

		entry.seen = false
	}
}

func (driver *Driver) releaseMaterials() {
	for id, entry := range driver.materials {
		entry.release()
		delete(driver.materials, id)
	}

	for source := range driver.textures {
		driver.UnloadTexture(source)
	}
}

func (entry *gpuMaterial) release() {
	entry.uniform.Call("destroy")

	if entry.impostor != nil {
		entry.impostor.release()
	}
}

func (entry *gpuMaterial) bindingsChanged(
	albedo, normal, occlusion, reflection, lightmap *texturedata.Texture,
	rnm [3]*texturedata.Texture,
) bool {
	return entry.group.IsUndefined() || entry.albedo != albedo || entry.normal != normal || entry.occlusion != occlusion ||
		entry.reflection != reflection ||
		entry.lightmap != lightmap || entry.rnm != rnm
}
