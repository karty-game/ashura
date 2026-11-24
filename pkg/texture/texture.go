// Package texture defines format-independent GPU-ready texture data and handles.
// Create textures with App.NewTexture and release them with App.UnloadTexture.
// Source-image decoding and preparation belong to the application or its build.
package texture

import "github.com/karty-game/ashura/internal/texturedata"

// Texture is an App-owned texture handle. Prepared textures are immutable;
// compute output textures may be written by scheduled GPU work.
type Texture = texturedata.Texture

// RNMLightmap binds three linear directional irradiance maps and their shared scale.
// Geometry must provide prepared secondary UVs.
type RNMLightmap = texturedata.RNMLightmap

// RGBLightmap stores the calibrated diffuse factor D=irradiance/pi. The
// material applies receiver albedo once. Preparation and encoding are offline.
type RGBLightmap struct {
	Texture  *Texture
	Scale    float32
	Encoding RNMEncoding
}

// RNMEncoding identifies a directional irradiance atlas's numeric encoding.
type RNMEncoding = texturedata.RNMEncoding

const (
	// RNMLinear samples normalized linear RGB decoded by the tuple scale.
	// Prepared textures may use hardware sRGB transfer for 8-bit precision.
	RNMLinear = texturedata.RNMLinear
	// RNMRGBE preserves dim irradiance with RGB mantissas and a biased exponent
	// in alpha. The Source shader decodes before bilinear interpolation.
	RNMRGBE = texturedata.RNMRGBE
)

// Data contains GPU-ready pixel or block mip levels. Zero Format selects RGBA8.
type Data = texturedata.Data

// Format selects the GPU payload representation, independently of color space.
type Format = texturedata.Format

const (
	RGBA8    = texturedata.RGBA8
	BC1      = texturedata.BC1
	ETC2RGB  = texturedata.ETC2RGB
	BC6H     = texturedata.BC6H
	BC7      = texturedata.BC7
	ETC2RGBA = texturedata.ETC2RGBA
	ASTC4x4  = texturedata.ASTC4x4
	ASTC6x6  = texturedata.ASTC6x6
)

// ColorSpace describes RGB encoding in the prepared payload.
type ColorSpace = texturedata.ColorSpace

const (
	// SRGB is for albedo/color textures.
	SRGB = texturedata.SRGB
	// Linear is for normal maps and other numeric textures.
	Linear = texturedata.Linear
)
