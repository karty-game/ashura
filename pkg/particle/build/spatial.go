//go:build !js

package build

import (
	"encoding/binary"
	"fmt"
	"math"

	"github.com/karty-game/ashura/internal/particledata"
)

// SpatialShape determines how a layer fills the world around its emitter.
type SpatialShape uint8

const (
	// Sphere spawns throughout a sphere and sends particles radially outward.
	Sphere SpatialShape = particledata.SpatialSphere
	// Cone spawns on a disc and spreads velocity around the authored cone direction.
	Cone SpatialShape = particledata.SpatialCone
	// GroundPlane maps the authored XY motion to world XZ.
	GroundPlane SpatialShape = particledata.SpatialGround
)

// SpatialLayer overrides emission and placement in a prepared effect.
type SpatialLayer struct {
	Shape         SpatialShape
	Radius        float64 // Sphere radius or starting cone disc radius in authored pixels.
	SpreadDegrees float64 // Full cone angle, from 0 to 180 degrees.
	HeightOffset  float64 // Upward offset in authored pixels for world placement.
	OffsetX       float64 // Horizontal offset in authored pixels.
	OffsetZ       float64 // Depth offset in authored pixels.
	RiseSpeed     float64 // Initial upward emitter velocity in authored pixels per second.
	RiseGravity   float64 // Downward acceleration of the moving emitter in authored pixels per second squared.
}

// SpatialLayers maps exact Particlr layer IDs to their spatial settings.
type SpatialLayers map[string]SpatialLayer

func packSpatial(config []byte, sourceShape string, settings SpatialLayer) error {
	const positionLimit, maxSpread, degreesPerTurn = 1e6, 180, 360
	if !between(settings.Radius, 0, positionLimit) || !between(settings.HeightOffset, -positionLimit, positionLimit) ||
		!between(settings.OffsetX, -positionLimit, positionLimit) || !between(settings.OffsetZ, -positionLimit, positionLimit) ||
		!between(settings.RiseSpeed, 0, positionLimit) || !between(settings.RiseGravity, 0, positionLimit) ||
		!between(settings.SpreadDegrees, 0, maxSpread) {
		return fmt.Errorf("spatial radius, height or spread out of bounds: %w", errDocument)
	}

	var cosine float64

	switch settings.Shape {
	case Sphere:
		if settings.SpreadDegrees != 0 {
			return fmt.Errorf("sphere cannot specify cone spread: %w", errDocument)
		}
	case Cone:
		if sourceShape != coneShape {
			return fmt.Errorf("spatial cone requires a Particlr cone direction: %w", errDocument)
		}

		cosine = math.Cos(settings.SpreadDegrees * math.Pi / degreesPerTurn)
	case GroundPlane:
		if settings.Radius != 0 || settings.SpreadDegrees != 0 {
			return fmt.Errorf("ground plane uses the authored planar radius and spread: %w", errDocument)
		}
	default:
		return fmt.Errorf("unknown spatial shape %d: %w", settings.Shape, errDocument)
	}

	for i, value := range [...]float64{float64(settings.Shape), settings.Radius, cosine, settings.HeightOffset} {
		binary.LittleEndian.PutUint32(config[particledata.SpatialOffset+i*4:], math.Float32bits(float32(value)))
	}

	for i, value := range [...]float64{settings.OffsetX, settings.OffsetZ, settings.RiseSpeed, settings.RiseGravity} {
		binary.LittleEndian.PutUint32(config[particledata.PlacementOffset+i*4:], math.Float32bits(float32(value)))
	}

	return nil
}

func packRender(config []byte, settings *renderSettings) error {
	if settings == nil {
		return nil
	}

	var mode float64

	switch settings.Align {
	case "none":
		mode = 1
	case "velocity":
		mode = 2
	default:
		return fmt.Errorf("particle render alignment %q: %w", settings.Align, errUnsupported)
	}

	if !between(settings.SpeedScale, 0, 1e6) ||
		!between(settings.MinStretch, .001, settings.MaxStretch) || settings.MaxStretch > particledata.MaxStretch {
		return fmt.Errorf("particle speed scale or stretch: %w", errDocument)
	}

	for i, value := range [...]float64{mode, settings.SpeedScale, settings.MinStretch, settings.MaxStretch} {
		binary.LittleEndian.PutUint32(config[particledata.RenderOffset+i*4:], math.Float32bits(float32(value)))
	}

	return nil
}
