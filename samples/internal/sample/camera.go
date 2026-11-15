package sample

import (
	"math"
	"time"

	"github.com/karty-game/karty-webgpu/pkg/camera"
	"github.com/karty-game/karty-webgpu/pkg/engine"
)

// FitCamera keeps a perspective comparison visible on narrow viewports without
// moving the camera out of its authored space. Only viewport changes update FOV.
//
//nolint:mnd // Perspective projection uses the standard half-angle FOV formula.
func FitCamera(app *engine.App, world *engine.World, entity engine.Entity) {
	projection, _ := engine.Get[camera.Perspective](world, entity)

	const (
		degrees            = 180 / math.Pi
		maximumFieldOfView = 120
	)

	halfAngle := float64(projection.FieldOfViewDegrees) / (2 * degrees)

	var previousWidth, previousHeight int

	app.OnUpdate(func(time.Duration) {
		width, height := app.ViewportSize()
		if width <= 0 || height <= 0 || width == previousWidth && height == previousHeight {
			return
		}

		previousWidth, previousHeight = width, height
		projection.FieldOfViewDegrees = min(
			maximumFieldOfView,
			float32(2*math.Atan(math.Tan(halfAngle)*max(1, float64(height)/float64(width)))*degrees),
		)
	})
}
