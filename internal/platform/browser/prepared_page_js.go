//go:build js && wasm

package browser

import (
	"syscall/js"

	"github.com/karty-game/karty-webgpu/internal/texturedata"
	"github.com/karty-game/karty-webgpu/internal/uipaint"
)

// Shared GPU declarations belong to prepared pages: sprites and retained UI
// consume the same upload ABI without linking the optional UI backend.
const (
	uiVertexBuffers = "buffers"
	uiInstanceStep  = "instance"
	uiFloat4Format  = "float32x4"
	uiTextureFormat = "rgba8unorm"
	uiAttachments   = "colorAttachments"
	uiStore         = "store"
)

type uiPage struct {
	texture, group js.Value
	used           uint64
}

const preparedTextureUsage = 0x06 // TEXTURE_BINDING | COPY_DST.

// UI and sprites upload the same prepared page format with distinct samplers.
func (driver *Driver) uploadPreparedPage(page *uipaint.Texture, pipeline, sampler js.Value) (uiPage, error) {
	format, err := driver.preparedPageFormat(page)
	if err != nil {
		return uiPage{}, err
	}

	rowBytes, rows := texturedata.Layout(page.Format, page.Width, page.Height)

	size := jsObject(map[string]any{widthProperty: page.Width, heightProperty: page.Height, depthOrArrayLayersProperty: 1})
	texture := driver.device.Call("createTexture", jsObject(map[string]any{
		sizeProperty: size, formatProperty: format, usageProperty: preparedTextureUsage,
	}))
	array := js.Global().Get("Uint8Array").New(len(page.Pixels))
	js.CopyBytesToJS(array, page.Pixels)
	driver.queue.Call("writeTexture", jsObject(map[string]any{"texture": texture}), array,
		jsObject(map[string]any{"bytesPerRow": rowBytes, "rowsPerImage": rows}), size)
	group := driver.device.Call("createBindGroup", jsObject(map[string]any{
		layoutProperty: pipeline.Call("getBindGroupLayout", 1), entriesProperty: []any{
			jsObject(map[string]any{bindingProperty: 0, resourceProperty: texture.Call("createView")}),
			jsObject(map[string]any{bindingProperty: 1, resourceProperty: sampler}),
		},
	}))

	return uiPage{texture: texture, group: group}, nil
}

func (driver *Driver) preparedPageFormat(page *uipaint.Texture) (string, error) {
	if page == nil || !uipaint.ValidPage(page.Width, page.Height, page.Format, len(page.Pixels)) {
		return "", errTextureUpload
	}

	if page.Format == texturedata.RGBA8 {
		return uiTextureFormat, nil
	}

	feature, format := gpuBlockFormat(page.Format)
	if !driver.device.Get("features").Call("has", feature).Bool() {
		return "", errTextureUpload
	}

	return format, nil
}
