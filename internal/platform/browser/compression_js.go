//go:build js && wasm

package browser

import (
	"syscall/js"

	"github.com/karty-game/ashura/internal/texturedata"
)

const (
	textureCompressionBC   = "texture-compression-bc"
	textureCompressionETC2 = "texture-compression-etc2"
	textureCompressionASTC = "texture-compression-astc"
)

type browserCompression struct{ bc, etc2, astc bool }

func enabledCompression(device js.Value) browserCompression {
	features := device.Get("features")

	return browserCompression{
		bc:   features.Call("has", textureCompressionBC).Bool(),
		etc2: features.Call("has", textureCompressionETC2).Bool(),
		astc: features.Call("has", textureCompressionASTC).Bool(),
	}
}

func (compression browserCompression) has(feature string) bool {
	switch feature {
	case textureCompressionBC:
		return compression.bc
	case textureCompressionETC2:
		return compression.etc2
	case textureCompressionASTC:
		return compression.astc
	default:
		return false
	}
}

// CompressedAlbedoFormat selects a format from the enabled-device snapshot.
func (driver *Driver) CompressedAlbedoFormat() texturedata.Format {
	if driver.compression.bc {
		return texturedata.BC1
	}

	if driver.compression.etc2 {
		return texturedata.ETC2RGB
	}

	return texturedata.RGBA8
}

func compressionFeatures(adapter js.Value) []any {
	var required []any

	features := adapter.Get("features")
	for _, feature := range []string{textureCompressionBC, textureCompressionETC2, textureCompressionASTC} {
		if features.Call("has", feature).Bool() {
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

		if !driver.compression.has(feature) {
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
	case texturedata.ASTC6x6:
		return textureCompressionASTC, "astc-6x6-unorm"
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

// SupportsTextureFormat reports enabled device capabilities without preparing data.
func (driver *Driver) SupportsTextureFormat(format texturedata.Format) bool {
	if format == texturedata.RGBA8 {
		return true
	}

	feature, _ := gpuBlockFormat(format)

	return driver.compression.has(feature)
}
