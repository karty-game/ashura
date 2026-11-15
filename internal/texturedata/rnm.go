package texturedata

// RNMEncoding identifies the numeric storage of directional irradiance.
type RNMEncoding uint8

const (
	// RNMLinear samples linear RGB multiplied by the tuple scale. The texture
	// may store linear values or use hardware sRGB decoding.
	RNMLinear RNMEncoding = iota
	// RNMRGBE stores normalized RGB mantissas and a shared biased base-two
	// exponent in alpha. Filtering must decode each texel before interpolation.
	RNMRGBE
)

// RNMLightmap contains linear irradiance for the three Source tangent-space
// radiosity normal mapping basis directions. Scale decodes their shared range.
// Textures are GPU-ready; no packing or conversion occurs while rendering.
type RNMLightmap struct {
	Textures [3]*Texture
	Scale    float32
	Encoding RNMEncoding
}
