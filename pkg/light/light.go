// Package light defines simple lighting components for entities.
package light

import (
	"github.com/karty-game/karty-webgpu/internal/render"
	engine "github.com/karty-game/karty-webgpu/pkg/engine"
)

const (
	defaultIntensity   float32 = 1
	defaultSkyRed      float32 = 0.65
	defaultSkyGreen    float32 = 0.75
	defaultGroundRed   float32 = 0.3
	defaultGroundGreen float32 = 0.27
	defaultGroundBlue  float32 = 0.22
)

// Hemisphere is an ambient fill light blending sky and ground colors according
// to surface orientation.
type Hemisphere struct {
	Name        string
	Direction   engine.Vector3
	SkyColor    engine.Color
	GroundColor engine.Color
	Intensity   float32
}

// HemisphereOptions configures a hemisphere light. Zero Direction selects +Y;
// zero SkyColor selects linear (0.65, 0.75, 1) and zero GroundColor selects
// (0.3, 0.27, 0.22). Zero Intensity selects one. Set the resulting component's
// Intensity to zero to disable it without removing the light.
type HemisphereOptions struct {
	Direction   engine.Vector3
	SkyColor    engine.Color
	GroundColor engine.Color
	Intensity   float32
}

// Directional is a sun-like light. Direction describes the direction light
// rays travel, from the source toward illuminated surfaces.
type Directional struct {
	Name      string
	Direction engine.Vector3
	Color     engine.Color
	Intensity float32
}

// DirectionalOptions configures a directional light. Zero Direction selects -Y,
// zero Color selects white and zero Intensity selects one. Direction describes
// light rays traveling toward surfaces; it need not have unit length.
// Set the resulting component's Intensity to zero to disable the light.
type DirectionalOptions struct {
	Direction engine.Vector3
	Color     engine.Color
	Intensity float32
}

// NewHemisphere creates a hemisphere light with default intensity one.
// Prefer DefaultHemisphere when zero-valued settings must remain explicit.
func NewHemisphere(name string, options HemisphereOptions) Hemisphere {
	if options.Intensity == 0 {
		options.Intensity = defaultIntensity
	}

	if options.Direction == (engine.Vector3{}) {
		options.Direction = engine.Vector3{Y: 1}
	}

	if options.SkyColor == (engine.Color{}) {
		options.SkyColor = engine.Color{R: defaultSkyRed, G: defaultSkyGreen, B: 1, A: 1}
	}

	if options.GroundColor == (engine.Color{}) {
		options.GroundColor = engine.Color{R: defaultGroundRed, G: defaultGroundGreen, B: defaultGroundBlue, A: 1}
	}

	return Hemisphere{
		Name:        name,
		Direction:   options.Direction,
		SkyColor:    options.SkyColor,
		GroundColor: options.GroundColor,
		Intensity:   options.Intensity,
	}
}

// RuntimeLight provides hemisphere values to the engine renderer.
func (source Hemisphere) RuntimeLight() (uint32, [3]float32, [3]float32, [3]float32, float32) {
	return render.HemisphereLightKind, vectorArray(
			source.Direction,
		), colorArray(
			source.SkyColor,
		), colorArray(
			source.GroundColor,
		), source.Intensity
}

// NewDirectional creates a directional light with default intensity one.
// Prefer DefaultDirectional when zero-valued settings must remain explicit.
func NewDirectional(name string, options DirectionalOptions) Directional {
	if options.Intensity == 0 {
		options.Intensity = defaultIntensity
	}

	if options.Direction == (engine.Vector3{}) {
		options.Direction = engine.Vector3{Y: -1}
	}

	if options.Color == (engine.Color{}) {
		options.Color = engine.Color{R: 1, G: 1, B: 1, A: 1}
	}

	return Directional{
		Name:      name,
		Direction: options.Direction,
		Color:     options.Color,
		Intensity: options.Intensity,
	}
}

// RuntimeLight provides directional values to the engine renderer.
func (source Directional) RuntimeLight() (uint32, [3]float32, [3]float32, [3]float32, float32) {
	return render.DirectionalLightKind, vectorArray(source.Direction), colorArray(source.Color), [3]float32{}, source.Intensity
}

func vectorArray(value engine.Vector3) [3]float32 { return [3]float32{value.X, value.Y, value.Z} }

func colorArray(value engine.Color) [3]float32 { return [3]float32{value.R, value.G, value.B} }
