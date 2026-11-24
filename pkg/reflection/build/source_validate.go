//go:build !js

package reflectionbuild

import (
	"fmt"
	"math"

	"github.com/karty-game/ashura/pkg/reflection/scene"
)

func validateSettings(source settings) error {
	if source.Space == "" || len(source.Space) > 128 || len(source.Objects) != 0 ||
		!validVector(source.ProbePosition) || !positiveVector(source.ProbeHalfExtents) ||
		!validVector(source.Fill.Direction) || vectorOf(source.Fill.Direction).dot(vectorOf(source.Fill.Direction)) == 0 ||
		!validVector(source.Key.Direction) || vectorOf(source.Key.Direction).dot(vectorOf(source.Key.Direction)) == 0 ||
		!validColor(source.Fill.SkyColor) || !validColor(source.Fill.GroundColor) || !validColor(source.Key.Color) ||
		!bounded(source.Fill.Intensity, maxCoordinate) || !bounded(source.Key.Intensity, maxCoordinate) {
		return errSource
	}

	return nil
}

func validateChunk(part chunk, owners, names map[string]bool, count int) error {
	if part.Space == "" || len(part.Space) > 128 || count > maxObjects || len(part.Objects) == 0 {
		return errSource
	}

	for _, coordinate := range part.Cell {
		if coordinate < -maxCoordinate/chunkSize || coordinate > maxCoordinate/chunkSize {
			return errSource
		}
	}

	owner := fmt.Sprintf("%s/%v", part.Space, part.Cell)
	if owners[owner] {
		return fmt.Errorf("duplicate chunk %s: %w", owner, errSource)
	}

	owners[owner] = true

	for _, object := range part.Objects {
		name := part.Space + "/" + object.Name
		if names[name] || !validObject(object) || !objectWithinCell(object, part.Cell) {
			return fmt.Errorf("object %q must be valid and contained in its chunk: %w", object.Name, errSource)
		}

		names[name] = true
	}

	return nil
}

func validObject(object reflectionscene.Object) bool {
	if object.Name == "" || len(object.Name) > 128 || !validVector(object.Position) || !validVector(object.Rotation) ||
		!validVector(object.Size) || !validColor(object.Surface.Color) || !bounded(object.Surface.Roughness, 1) ||
		!bounded(object.Surface.ReflectionStrength, 1) {
		return false
	}

	switch object.Shape {
	case reflectionscene.Plane:
		return object.Size.X > 0 && object.Size.Y == 0 && object.Size.Z > 0
	case reflectionscene.Box:
		return positiveVector(object.Size)
	case reflectionscene.Sphere:
		return object.Size.X > 0 && object.Size.Y == 0 && object.Size.Z == 0
	default:
		return false
	}
}

func objectWithinCell(object reflectionscene.Object, cell [3]int) bool {
	half := vectorOf(object.Size).scale(.5)
	if object.Shape == reflectionscene.Sphere {
		half = vector{half[0], half[0], half[0]}
	}

	primitive := primitives([]reflectionscene.Object{object})[0]
	position := vectorOf(object.Position)

	for axis, coordinate := range position {
		extent := half[0]*math.Abs(primitive.axes[0][axis]) + half[1]*math.Abs(primitive.axes[1][axis]) +
			half[2]*math.Abs(primitive.axes[2][axis])

		minimum := float64(cell[axis])*chunkSize + chunkOrigin
		if coordinate-extent < minimum-rayEpsilon || coordinate+extent > minimum+chunkSize+rayEpsilon {
			return false
		}
	}

	return true
}

func validVector(value reflectionscene.Vector3) bool {
	for _, coordinate := range []float32{value.X, value.Y, value.Z} {
		if !bounded(coordinate+maxCoordinate, 2*maxCoordinate) {
			return false
		}
	}

	return true
}

func positiveVector(value reflectionscene.Vector3) bool {
	return validVector(value) && value.X > 0 && value.Y > 0 && value.Z > 0
}

func validColor(value reflectionscene.Color) bool {
	return bounded(value.R, 1) && bounded(value.G, 1) && bounded(value.B, 1) && value.A == 1
}

func bounded(value, maximum float32) bool {
	return !math.IsNaN(float64(value)) && !math.IsInf(float64(value), 0) && value >= 0 && value <= maximum
}
