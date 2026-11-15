package material

import (
	"github.com/karty-game/karty-webgpu/pkg/engine"
	"github.com/karty-game/karty-webgpu/pkg/texture"
)

// SourceOptions configures a diffuse material inspired by Source's RNM/SSBump shading.
// RNMLightmap contains combined static direct, environment and bounced irradiance.
// AmbientColors supply a fallback when no map is attached; zero colors select
// a neutral fill in the three fixed tangent-space RNM directions.
type SourceOptions struct {
	Color         engine.Color
	AlbedoTexture *texture.Texture
	SSBumpTexture *texture.Texture
	RNMLightmap   *texture.RNMLightmap
	AmbientColors [3]engine.Color
	UVScale       engine.Vector2
	// CullBackfaces discards clockwise triangles. The default renders both sides.
	CullBackfaces bool
}

// SourceMaterial uses unsigned RGB SSBump coefficients instead of normal/AO maps.
// Coefficient order is (+sqrt(2/3),0,1/sqrt(3)),
// (-1/sqrt(6),+1/sqrt(2),1/sqrt(3)), (-1/sqrt(6),-1/sqrt(2),1/sqrt(3)).
// AmbientColors supply irradiance along these tangent-space directions, not world axes.
// Coefficients are never normalized: their magnitude preserves baked occlusion.
// Attach one material component per entity.
type SourceMaterial struct {
	Color         engine.Color
	AlbedoTexture *texture.Texture
	SSBumpTexture *texture.Texture
	RNMLightmap   *texture.RNMLightmap
	AmbientColors [3]engine.Color
	UVScale       engine.Vector2
	// CullBackfaces discards clockwise triangles. The default renders both sides.
	CullBackfaces bool
	// HalfLambert enables an optional wrapped-light comparison. The default uses
	// Source LightmappedGeneric's direct SSBump coefficient projection.
	HalfLambert bool
	// SSBumpStrength blends from a flat, unoccluded surface (zero) to the map (one).
	SSBumpStrength float32
}

// Source creates an opaque diffuse RNM material with direct SSBump lighting.
// Directional lights project onto the RNM basis and weight the stored SSBump
// coefficients without normalizing them. Hemisphere lights are ignored; use
// AmbientColors for ambient lighting. No textures are transformed at runtime.
// The documented zero-as-default options remain for compatibility. Prefer
// DefaultSource when constructing direct-only or disabled-map materials.
func Source(options SourceOptions) SourceMaterial {
	if options.Color == (engine.Color{}) {
		options.Color = engine.ColorWhite()
	}

	if options.UVScale == (engine.Vector2{}) {
		options.UVScale = engine.Vector2{X: 1, Y: 1}
	}

	if options.AmbientColors == ([3]engine.Color{}) {
		const fill = .2

		color := engine.Color{R: fill, G: fill, B: fill, A: 1}
		options.AmbientColors = [3]engine.Color{color, color, color}
	}

	return SourceMaterial{
		Color: options.Color, AlbedoTexture: options.AlbedoTexture,
		SSBumpTexture: options.SSBumpTexture, AmbientColors: options.AmbientColors,
		RNMLightmap: options.RNMLightmap,
		UVScale:     options.UVScale, SSBumpStrength: 1,
		CullBackfaces: options.CullBackfaces,
	}
}

// RuntimeMaterial supplies the tint; Source shading has no specular term.
func (value SourceMaterial) RuntimeMaterial() (engine.Color, float32) { return value.Color, 1 }

// RuntimeCullBackfaces selects the cached pipeline's optional back-face culling.
func (value SourceMaterial) RuntimeCullBackfaces() bool { return value.CullBackfaces }

// RuntimeTextures supplies albedo and linear SSBump coefficients to the Source pipeline.
func (value SourceMaterial) RuntimeTextures() (*texture.Texture, *texture.Texture, engine.Vector2, float32) {
	return value.AlbedoTexture, value.SSBumpTexture, value.UVScale, 0
}

// RuntimeSource supplies the RNM lighting and Source-specific shading settings.
func (value SourceMaterial) RuntimeSource() ([3]engine.Color, bool, float32) {
	return value.AmbientColors, value.HalfLambert, value.SSBumpStrength
}

// RuntimeRNMLightmap supplies combined baked direct, environment and bounced
// irradiance. When attached, it replaces AmbientColors.
func (value SourceMaterial) RuntimeRNMLightmap() *texture.RNMLightmap { return value.RNMLightmap }
