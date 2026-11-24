// Package material provides constructors for engine material values.
package material

import (
	engine "github.com/karty-game/ashura/pkg/engine"
	"github.com/karty-game/ashura/pkg/texture"
)

// Options is the compatibility input to Standard. It has the same fields as
// Material, but Standard applies its documented zero-as-default rules. Prefer
// DefaultStandard and set component fields directly when zero is meaningful.
type Options = Material

// Material is the by-value standard shading component attached to an entity.
type Material struct {
	Color     engine.Color
	Roughness float32
	// ReflectionStrength blends in the active local probe from zero to one.
	ReflectionStrength float32
	// CullBackfaces discards clockwise triangles. The default renders both sides.
	CullBackfaces bool
	// AlphaMask discards Standard fragments below AlphaCutoff without blending.
	AlphaMask bool
	// AlphaCutoff compares albedo alpha times Color.A; zero is meaningful.
	// Masked cutoff and tint alpha must be finite values in [0,1].
	AlphaCutoff float32
	// AlbedoTexture is sampled as sRGB and multiplied by Color.
	AlbedoTexture *texture.Texture
	// NormalTexture stores tangent-space RGB normals, loaded as texture.Linear.
	NormalTexture *texture.Texture
	// AOTexture uses the red channel of a linear ambient-occlusion map.
	AOTexture *texture.Texture
	// LightmapTexture contains linear diffuse irradiance sampled with prepared UV1.
	LightmapTexture *texture.Texture
	// RGBLightmap selects calibrated diffuse lighting produced by lightmap/bake.
	// It takes precedence over the retained scalar LightmapTexture fields.
	RGBLightmap *texture.RGBLightmap
	// LightmapScale decodes the prepared irradiance range; zero disables it.
	LightmapScale float32
	// LightmapDebug displays a secondary-UV checker instead of shaded color.
	LightmapDebug bool
	// AOStrength defaults to one; set the material field to zero to disable AO.
	AOStrength float32
	// UVScale repeats the maps; zero defaults to (1,1).
	UVScale engine.Vector2
	// NormalStrength defaults to one. Set the material field to zero to disable mapping.
	NormalStrength float32
}

// Standard creates a lightweight diffuse-plus-specular material descriptor.
// It does not imply a full physically based material model. Zero Color selects
// white; zero UVScale selects (1,1); zero NormalStrength and AOStrength select one.
// These compatibility defaults are applied only here. Prefer DefaultStandard
// when an explicit zero strength is required during construction.
func Standard(options Options) Material {
	color := options.Color
	if color == (engine.Color{}) {
		color = engine.Color{R: 1, G: 1, B: 1, A: 1}
	}

	if options.UVScale == (engine.Vector2{}) {
		options.UVScale = engine.Vector2{X: 1, Y: 1}
	}

	if options.NormalStrength == 0 {
		options.NormalStrength = 1
	}

	if options.AOStrength == 0 {
		options.AOStrength = 1
	}

	options.Color = color

	return options
}

// RuntimeMaterial provides the stored shading values to the engine renderer.
func (value Material) RuntimeMaterial() (engine.Color, float32) {
	return value.Color, value.Roughness
}

// RuntimeAlphaMask supplies Standard's optional opaque cutout settings.
func (value Material) RuntimeAlphaMask() (bool, float32) { return value.AlphaMask, value.AlphaCutoff }

// RuntimeReflectionStrength opts this standard material into local probes.
func (value Material) RuntimeReflectionStrength() float32 { return value.ReflectionStrength }

// RuntimeCullBackfaces selects the cached pipeline's optional back-face culling.
func (value Material) RuntimeCullBackfaces() bool { return value.CullBackfaces }

// RuntimeTextures supplies immutable textures and sampling settings to the renderer.
func (value Material) RuntimeTextures() (*texture.Texture, *texture.Texture, engine.Vector2, float32) {
	return value.AlbedoTexture, value.NormalTexture, value.UVScale, value.NormalStrength
}

// RuntimeOcclusion supplies linear ambient-occlusion data to the renderer.
func (value Material) RuntimeOcclusion() (*texture.Texture, float32) {
	return value.AOTexture, value.AOStrength
}

// RuntimeLightmap supplies prepared linear irradiance and its decoding scale.
func (value Material) RuntimeLightmap() (*texture.Texture, float32, bool) {
	if value.RGBLightmap != nil {
		return value.RGBLightmap.Texture, value.RGBLightmap.Scale, value.LightmapDebug
	}

	return value.LightmapTexture, value.LightmapScale, value.LightmapDebug
}

// RuntimeLightmapEncoding describes the prepared numeric encoding and D units.
func (value Material) RuntimeLightmapEncoding() (texture.RNMEncoding, bool) {
	if value.RGBLightmap != nil {
		return value.RGBLightmap.Encoding, true
	}

	return texture.RNMLinear, false
}
