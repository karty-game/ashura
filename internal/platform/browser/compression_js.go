//go:build js && wasm

package browser

import (
	"syscall/js"

	"github.com/karty-game/karty-webgpu/internal/texturedata"
)

const (
	textureCompressionBC   = "texture-compression-bc"
	textureCompressionETC2 = "texture-compression-etc2"
	textureCompressionASTC = "texture-compression-astc"
)

// CompressedAlbedoFormat queries the enabled device once per residency request.
func (driver *Driver) CompressedAlbedoFormat() texturedata.Format {
	features := driver.device.Get("features")
	if features.Call("has", textureCompressionBC).Bool() {
		return texturedata.BC1
	}

	if features.Call("has", textureCompressionETC2).Bool() {
		return texturedata.ETC2RGB
	}

	return texturedata.RGBA8
}

func compressionFeatures(adapter js.Value) []any {
	var required []any

	for _, feature := range []string{textureCompressionBC, textureCompressionETC2, textureCompressionASTC} {
		if adapter.Get("features").Call("has", feature).Bool() {
			required = append(required, feature)
		}
	}

	return required
}
func (driver *Driver) preparedTextureFormat(source *texturedata.Texture) (string, error) {
	format := "rgba8unorm"
	if texturedata.Space(source) == texturedata.SRGB {
		format = "rgba8unorm-srgb"
	}

	encoding := texturedata.Encoding(source)
	if encoding != texturedata.RGBA8 {
		feature, blockFormat := gpuBlockFormat(encoding)
		format = blockFormat

		if !driver.device.Get("features").Call("has", feature).Bool() {
			return "", errTextureUpload
		}

		if texturedata.Space(source) == texturedata.SRGB {
			format += "-srgb"
		}
	}

	return format, nil
}

func gpuBlockFormat(encoding texturedata.Format) (string, string) {
	switch encoding {
	case texturedata.ETC2RGB:
		return textureCompressionETC2, "etc2-rgb8unorm"
	case texturedata.ETC2RGBA:
		return textureCompressionETC2, "etc2-rgba8unorm"
	case texturedata.ASTC4x4:
		return textureCompressionASTC, "astc-4x4-unorm"
	case texturedata.BC6H:
		return textureCompressionBC, "bc6h-rgb-ufloat"
	case texturedata.BC7:
		return textureCompressionBC, "bc7-rgba-unorm"
	case texturedata.BC1:
		return textureCompressionBC, "bc1-rgba-unorm"
	case texturedata.RGBA8:
		return "", uiTextureFormat
	default:
		return "", ""
	}
}
