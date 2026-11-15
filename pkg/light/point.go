package light

import (
	"github.com/karty-game/karty-webgpu/internal/render"
	"github.com/karty-game/karty-webgpu/pkg/engine"
)

// Point is an omnidirectional light with finite influence. Position is in world
// units and does not depend on an entity Transform. Change fields to move or
// disable it. The first shadow slice supports one shadowed point light per world.
type Point struct {
	Name             string
	Position         engine.Vector3
	Color            engine.Color
	Intensity, Range float32
	// CastShadows must be enabled before Run to prepare shadow pipelines.
	CastShadows bool
	DebugFaces  bool
	// ShadowResolution is fixed for the owning App lifetime after Run starts.
	ShadowResolution uint32
	// ShadowBias offsets the depth comparison in world units.
	ShadowBias float32
}

// PointOptions configures a finite point light. Zero color selects white, zero
// intensity selects one, zero range selects 20, zero shadow resolution selects
// 512 and zero shadow bias selects 0.015 world units. Set Intensity to zero on
// the component to disable it after construction. Shadows are opt-in.
type PointOptions struct {
	Position         engine.Vector3
	Color            engine.Color
	Intensity, Range float32
	// CastShadows must be enabled before Run to prepare shadow pipelines.
	CastShadows bool
	DebugFaces  bool
	// ShadowResolution is fixed for the owning App lifetime after Run starts.
	ShadowResolution uint32
	ShadowBias       float32
}

// NewPoint creates a point light. Range bounds a smooth squared falloff:
// (1 - distance*distance/(Range*Range)) squared, zero outside Range.
// Prefer DefaultPoint when zero intensity or shadow bias must remain explicit.
func NewPoint(name string, options PointOptions) Point {
	if options.Color == (engine.Color{}) {
		options.Color = engine.ColorWhite()
	}

	if options.Intensity == 0 {
		options.Intensity = 1
	}

	if options.Range == 0 {
		options.Range = 20
	}

	if options.ShadowResolution == 0 {
		options.ShadowResolution = 512
	}

	if options.ShadowBias == 0 {
		options.ShadowBias = .015
	}

	return Point{
		Name:             name,
		Position:         options.Position,
		Color:            options.Color,
		Intensity:        options.Intensity,
		Range:            options.Range,
		CastShadows:      options.CastShadows,
		DebugFaces:       options.DebugFaces,
		ShadowResolution: options.ShadowResolution,
		ShadowBias:       options.ShadowBias,
	}
}

// RuntimeLight supplies the point position and range in the shared light record.
func (source Point) RuntimeLight() (uint32, [3]float32, [3]float32, [3]float32, float32) {
	shadow := float32(0)
	if source.CastShadows && source.Intensity > 0 {
		shadow = 1
	}

	return render.PointLightKind, vectorArray(
			source.Position,
		), colorArray(
			source.Color,
		), [3]float32{
			source.Range,
			shadow,
			0,
		}, source.Intensity
}

// RuntimePointShadow supplies explicit shadow admission and cache settings.
func (source Point) RuntimePointShadow() (bool, uint32, float32) {
	return source.CastShadows, source.ShadowResolution, source.ShadowBias
}

// RuntimePointShadowDebug opts into the six-face depth diagnostic overlay.
func (source Point) RuntimePointShadowDebug() bool { return source.DebugFaces }
