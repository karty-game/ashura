package ktx2

import (
	"fmt"

	"github.com/karty-game/karty-webgpu/pkg/engine"
	"github.com/karty-game/karty-webgpu/pkg/texture"
)

// Asset is an explicit reference to embedded, prepared texture bytes. Its path
// is build metadata and diagnostic identity, never a runtime filesystem lookup.
type Asset struct {
	path       string
	prepared   []byte
	space      texture.ColorSpace
	reflection bool
}

// Albedo declares an sRGB albedo asset. Use a literal path and an explicit embed
// variable so build tooling can discover its source and intended use.
func Albedo(path string, prepared []byte) Asset {
	return Asset{path: path, prepared: prepared, space: texture.SRGB}
}

// Reflection declares an sRGB six-face horizontal cubemap atlas with mips.
func Reflection(path string, prepared []byte) Asset {
	return Asset{path: path, prepared: prepared, space: texture.SRGB, reflection: true}
}

// Normal declares a linear tangent-space normal asset using prepared KTX2 data.
func Normal(path string, prepared []byte) Asset {
	return Asset{path: path, prepared: prepared, space: texture.Linear}
}

// SSBump declares three linear, unsigned RNM visibility coefficients in RGB.
func SSBump(path string, prepared []byte) Asset {
	return Asset{path: path, prepared: prepared, space: texture.Linear}
}

// Lightmap declares linear RGB diffuse irradiance. Its accompanying bake
// metadata supplies the scale applied after sampling; albedo is not baked in.
func Lightmap(path string, prepared []byte) Asset {
	return Asset{path: path, prepared: prepared, space: texture.Linear}
}

// Load creates an App-owned handle from this declared asset. No files are opened.
func (asset Asset) Load(app *engine.App) (*texture.Texture, error) {
	data, err := Decode(asset.prepared)
	if err != nil {
		return nil, fmt.Errorf("asset %s: %w", asset.path, err)
	}

	if data.ColorSpace != asset.space {
		return nil, fmt.Errorf("asset %s role/encoding mismatch: %w", asset.path, errFormat)
	}

	if asset.reflection && (data.Width%6 != 0 || data.Width/6 != data.Height) {
		return nil, fmt.Errorf("asset %s cubemap atlas dimensions: %w", asset.path, errFormat)
	}

	return app.NewTexture(data)
}

// AmbientOcclusion declares a linear red-channel ambient-occlusion asset.
func AmbientOcclusion(path string, prepared []byte) Asset {
	return Asset{path: path, prepared: prepared, space: texture.Linear}
}
