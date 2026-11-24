package render

import _ "embed"

//go:embed wgsl/standard.wgsl
var standardShader string

//go:embed wgsl/source.wgsl
var sourceShader string

//go:embed wgsl/shadow-sampling.wgsl
var shadowSamplingShader string

//go:embed wgsl/impostor.wgsl
var impostorShader string

// ImpostorShader returns the shared prepared impostor shader.
func ImpostorShader() string { return impostorShader }

// StandardShader returns only the standard material's complete WGSL source.
func StandardShader() string { return standardShader + "\n" + shadowSamplingShader }

// SourceShader returns only the Source material's complete WGSL source.
func SourceShader() string { return sourceShader + "\n" + shadowSamplingShader }

// FragmentEntry selects the file's output entry point for the presentation format.
func FragmentEntry(encodeSRGB bool) string {
	if encodeSRGB {
		return "fs_main_srgb"
	}

	return "fs_main"
}

//go:embed wgsl/point-shadow-debug.wgsl
var pointShadowDebugShader string

// PointShadowDebugShader returns the authored six-face diagnostic overlay.
func PointShadowDebugShader() string { return pointShadowDebugShader }

//go:embed wgsl/shadow-depth.wgsl
var shadowDepthShader string

// ShadowDepthShader writes projected depth for every local light view.
func ShadowDepthShader() string { return shadowDepthShader }
