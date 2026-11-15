package render

import _ "embed"

//go:embed wgsl/standard.wgsl
var standardShader string

//go:embed wgsl/source.wgsl
var sourceShader string

//go:embed wgsl/impostor.wgsl
var impostorShader string

// ImpostorShader returns the shared prepared impostor shader.
func ImpostorShader() string { return impostorShader }

// StandardShader returns only the standard material's complete WGSL source.
func StandardShader() string { return standardShader }

// SourceShader returns only the Source material's complete WGSL source.
func SourceShader() string { return sourceShader }

// FragmentEntry selects the file's output entry point for the presentation format.
func FragmentEntry(encodeSRGB bool) string {
	if encodeSRGB {
		return "fs_main_srgb"
	}

	return "fs_main"
}

//go:embed wgsl/point-shadow.wgsl
var pointShadowShader string

// PointShadowShader returns the authored six-face radial-depth shader.
func PointShadowShader() string { return pointShadowShader }

//go:embed wgsl/point-shadow-debug.wgsl
var pointShadowDebugShader string

// PointShadowDebugShader returns the authored six-face diagnostic overlay.
func PointShadowDebugShader() string { return pointShadowDebugShader }
