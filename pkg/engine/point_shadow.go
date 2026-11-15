package engine

import "github.com/karty-game/karty-webgpu/internal/render"

// Shadow pipeline/device work belongs to startup, before any animation callback.
// Disabled declared sources are included so Live/Baked mode changes need no GPU
// pipeline creation. The first slice pins one resolution for the App lifetime.
func pointShadowResolution(world *World) (uint32, error) {
	var frame render.Frame

	world.Each(func(entity Entity) { appendEntityLights(&frame, world, entity) })

	var resolution uint32

	for index := range frame.Lights {
		light := &frame.Lights[index]
		if light.Kind != render.PointLightKind || !light.CastShadows {
			continue
		}

		light.Intensity = 1
		resolution = light.ShadowResolution
	}

	if err := validateFrame(frame); err != nil {
		return 0, err
	}

	return resolution, nil
}
