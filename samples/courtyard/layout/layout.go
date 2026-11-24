// Package layout shares the courtyard blockout between its recipe and scene.
package layout

import (
	"github.com/karty-game/ashura/pkg/collision"
	"github.com/karty-game/ashura/pkg/engine"
)

const (
	// SpawnZ puts the player in the entry yard facing the first gate.
	SpawnZ     = 8
	firstGateZ = 3
	timedGateZ = -4.5
)

// Block creates a box from its center and dimensions.
func Block(centerX, centerY, centerZ, width, height, depth float32) collision.Bounds {
	const halfDimension = 0.5

	half := engine.Vector3{X: width * halfDimension, Y: height * halfDimension, Z: depth * halfDimension}

	return collision.Bounds{
		Min: engine.Vector3{X: centerX - half.X, Y: centerY - half.Y, Z: centerZ - half.Z},
		Max: engine.Vector3{X: centerX + half.X, Y: centerY + half.Y, Z: centerZ + half.Z},
	}
}

// Floor spans all three yards.
func Floor() collision.Bounds { return Block(0, -.25, 0, 16, .5, 26) }

// Stone returns solid walls and obstacles around the routes.
func Stone() []collision.Bounds { return append(append(Perimeter(), Dividers()...), Crates()...) }

// Perimeter encloses all three yards.
func Perimeter() []collision.Bounds {
	return []collision.Bounds{
		Block(-8, 1.1, 0, .5, 2.2, 26), Block(8, 1.1, 0, .5, 2.2, 26),
		Block(0, 1.1, -13, 16, 2.2, .5), Block(0, 1.1, 13, 16, 2.2, .5),
	}
}

// Dividers split the course into three yards.
func Dividers() []collision.Bounds {
	return []collision.Bounds{
		Block(-4.65, 1.15, firstGateZ, 6.7, 2.3, .7), Block(4.65, 1.15, firstGateZ, 6.7, 2.3, .7),
		Block(-4.65, 1.15, timedGateZ, 6.7, 2.3, .7), Block(4.65, 1.15, timedGateZ, 6.7, 2.3, .7),
	}
}

// Crates provides the cover obstacles and their prepared collider footprints.
func Crates() []collision.Bounds {
	return []collision.Bounds{
		Block(-3.7, .65, 4.9, 1.1, 1.3, 1.1), Block(3.7, .65, 4.9, 1.1, 1.3, 1.1),
		Block(0, .72, -.3, 1.65, 1.44, 1.65),
		Block(-2.3, .65, -2.4, .9, 1.3, .9), Block(2.3, .65, -.35, .9, 1.3, .9),
		Block(-2.8, .7, -8.5, 1.05, 1.4, 1.05), Block(2.8, .7, -8.5, 1.05, 1.4, 1.05),
	}
}

// FirstGate separates the entry and middle yards.
func FirstGate() collision.Bounds { return Block(0, 1.05, firstGateZ, 2.6, 2.1, .36) }

// TimedGate separates the middle and exit yards.
func TimedGate() collision.Bounds { return Block(0, 1.05, timedGateZ, 2.6, 2.1, .36) }

// Switch sits proud of the first gate's south-facing wall.
func Switch() collision.Bounds { return Block(-1.85, 1.42, 3.47, .54, .68, .24) }

// Plates can raise the timed gate from either side.
func Plates() [2]collision.Bounds {
	return [2]collision.Bounds{Block(3.4, .026, -2.25, 1.65, .052, 1.65), Block(-3.4, .026, -7, 1.65, .052, 1.65)}
}

// Finish returns the marked exit in the back yard.
func Finish() collision.Bounds { return Block(0, .025, -11.05, 2.7, .05, 2.2) }

// Relics provides two alternative routes through the same geometry.
//
//nolint:mnd // The two authored routes use explicit world-space coordinates.
func Relics() [2][3]engine.Vector3 {
	return [2][3]engine.Vector3{
		{{X: -2.85, Y: .65, Z: 6.2}, {X: 4.85, Y: .65, Z: .25}, {X: -4.4, Y: .65, Z: -9.5}},
		{{X: 2.85, Y: .65, Z: 6.2}, {X: -4.85, Y: .65, Z: -.8}, {X: 4.4, Y: .65, Z: -9.5}},
	}
}
