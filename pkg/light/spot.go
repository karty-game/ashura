package light

import (
	"math"

	"github.com/karty-game/ashura/pkg/engine"
)

// Spot is a finite-range cone light. Direction follows the outgoing beam;
// InnerAngle and OuterAngle are half-angles in radians.
type Spot struct {
	Name                   string
	Position, Direction    engine.Vector3
	Color                  engine.Color
	Intensity, Range       float32
	InnerAngle, OuterAngle float32
	// CastShadows reserves one of the clustered renderer's two spotlight maps.
	CastShadows bool
}

// DefaultSpot returns a white, downward-facing spotlight with shadows disabled.
// Set Intensity to zero before World.Add to start disabled.
func DefaultSpot(name string) Spot {
	const spotRange, innerDivisor, outerDivisor = 8, 8, 5

	return Spot{
		Name: name, Direction: engine.Vector3{Y: -1},
		Color: engine.Color{R: 1, G: 1, B: 1, A: 1}, Intensity: 1, Range: spotRange,
		InnerAngle: math.Pi / innerDivisor, OuterAngle: math.Pi / outerDivisor,
	}
}

// LightData supplies the current cone and radiance to the engine.
func (source Spot) LightData() engine.LightData {
	return engine.LightData{
		Kind: engine.SpotLight, Position: source.Position, Direction: source.Direction,
		Color: source.Color, Intensity: source.Intensity, Range: source.Range,
		InnerAngle: source.InnerAngle, OuterAngle: source.OuterAngle,
		Shadow: engine.PointShadow{Enabled: source.CastShadows},
	}
}
