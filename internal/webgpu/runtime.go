//go:build !js

package wgpu

// Native builds select -tags=nofakecgo so GoFFI and SDL share one callback
// runtime. Retain the provider in headless clients which never import SDL.
import _ "github.com/ebitengine/purego"
