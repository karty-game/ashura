//go:build !js

package reflectionbuild

import (
	"math"

	"github.com/karty-game/karty-webgpu/pkg/reflection/scene"
)

const rayEpsilon = 1e-6

type vector [3]float64

func vectorOf(value reflectionscene.Vector3) vector {
	return vector{float64(value.X), float64(value.Y), float64(value.Z)}
}

func (value vector) dot(other vector) float64 {
	return value[0]*other[0] + value[1]*other[1] + value[2]*other[2]
}

func (value vector) scale(factor float64) vector {
	return vector{value[0] * factor, value[1] * factor, value[2] * factor}
}

func (value vector) add(other vector) vector {
	return vector{value[0] + other[0], value[1] + other[1], value[2] + other[2]}
}

func (value vector) unit() vector { return value.scale(1 / math.Sqrt(value.dot(value))) }

// rotate follows engine.Transform's Rz * Ry * Rx Euler convention.
func rotate(value vector, angles reflectionscene.Vector3) vector {
	sine, cosine := math.Sincos(float64(angles.X))
	value = vector{value[0], cosine*value[1] - sine*value[2], sine*value[1] + cosine*value[2]}
	sine, cosine = math.Sincos(float64(angles.Y))
	value = vector{cosine*value[0] + sine*value[2], value[1], -sine*value[0] + cosine*value[2]}
	sine, cosine = math.Sincos(float64(angles.Z))

	return vector{cosine*value[0] - sine*value[1], sine*value[0] + cosine*value[1], value[2]}
}

type primitive struct {
	object reflectionscene.Object
	axes   [3]vector
}

func primitives(objects []reflectionscene.Object) []primitive {
	result := make([]primitive, len(objects))
	for index, object := range objects {
		result[index] = primitive{object: object, axes: [3]vector{
			rotate(vector{1, 0, 0}, object.Rotation),
			rotate(vector{0, 1, 0}, object.Rotation),
			rotate(vector{0, 0, 1}, object.Rotation),
		}}
	}

	return result
}

func (value primitive) intersect(origin, direction vector) (distance float64, normal vector) {
	offset := origin.add(vectorOf(value.object.Position).scale(-1))
	localOrigin := vector{offset.dot(value.axes[0]), offset.dot(value.axes[1]), offset.dot(value.axes[2])}
	localDirection := vector{direction.dot(value.axes[0]), direction.dot(value.axes[1]), direction.dot(value.axes[2])}
	half := vectorOf(value.object.Size).scale(.5)

	switch value.object.Shape {
	case reflectionscene.Plane:
		distance = face(localOrigin, localDirection, half, 1, 1)
		normal = value.axes[1]
	case reflectionscene.Sphere:
		projection := localOrigin.dot(localDirection)

		discriminant := projection*projection - localOrigin.dot(localOrigin) + half[0]*half[0]
		if discriminant < 0 {
			return math.Inf(1), vector{}
		}

		distance = -projection - math.Sqrt(discriminant)
		if distance <= rayEpsilon {
			return math.Inf(1), vector{}
		}

		normal = offset.add(direction.scale(distance)).unit()
	case reflectionscene.Box:
		distance = math.Inf(1)

		for axis := range 3 {
			for _, sign := range []float64{-1, 1} {
				candidate := face(localOrigin, localDirection, half, axis, sign)
				if candidate < distance {
					distance, normal = candidate, value.axes[axis].scale(sign)
				}
			}
		}
	default:
		panic("unknown authored room primitive")
	}

	return distance, normal
}

// face intersects one front-facing rectangular face, including the plane's +Y face.
func face(origin, direction, half vector, axis int, sign float64) float64 {
	if direction[axis]*sign >= -rayEpsilon {
		return math.Inf(1)
	}

	distance := (half[axis]*sign - origin[axis]) / direction[axis]
	if distance <= rayEpsilon {
		return math.Inf(1)
	}

	point := origin.add(direction.scale(distance))
	for other := range 3 {
		if other != axis && math.Abs(point[other]) > half[other]+rayEpsilon {
			return math.Inf(1)
		}
	}

	return distance
}
