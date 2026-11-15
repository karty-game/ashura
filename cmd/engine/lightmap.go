//go:build !js

package main

import (
	"context"
	"errors"
)

var errLegacyLightmapCommand = errors.New(
	"engine assets lightmap is retired: .klightmap uses the legacy bake contract; use LightmapUVs(Production: true) and ProductionBake in an asset recipe with the in-process Go WebGPU baker, then run engine assets build",
)

// buildLightmap retains the retired command only to report migration guidance.
// It rejects before parsing paths, reading input or starting an executor.
func buildLightmap(_ context.Context, _ []string) error {
	return errLegacyLightmapCommand
}
