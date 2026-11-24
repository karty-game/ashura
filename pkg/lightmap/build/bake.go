// Package lightmapbuild retains historical explicit browser lightmap APIs.
// Maintained offline builds use pkg/lightmap/bake and the native production worker.
package lightmapbuild

import (
	"context"
	"errors"
	"time"

	"github.com/karty-game/ashura/pkg/lightmap"
	"github.com/karty-game/ashura/pkg/texture"
)

// Result contains a one-mip linear RGBA8 atlas and its irradiance decoding scale.
// Duration includes device creation, uploads, pipeline creation and readback.
type Result struct {
	Texture  texture.Data
	Scale    float32
	Adapter  string
	Duration time.Duration
}

var errNativeUnavailable = errors.New(
	"legacy .klightmap native baking is unavailable; migrate the asset recipe to LightmapUVs(Production: true) and ProductionBake with the in-process Go WebGPU baker, then run engine assets build",
)

// Bake retains the historical unsupported native entry point. ProductionBake
// uses a different prepared-scene and calibrated lighting contract; legacy
// .klightmap artifacts are not implicitly converted.
func Bake(_ context.Context, _ *lightmap.Asset, _ lightmap.Point) (Result, error) {
	return Result{}, errNativeUnavailable
}
