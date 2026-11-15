//go:build !js

//nolint:varnamelen // Cofactor algebra uses conventional matrix and coordinate symbols.
package levelbuild

import (
	"encoding/binary"
	"math"
)

type transform struct {
	world  [16]float64
	normal [9]float64
}

func makeTransform(input [16]float32) (transform, error) {
	if input == ([16]float32{}) {
		input = [16]float32{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}
	}

	var output transform

	for index, value := range input {
		if !finite(float64(value)) {
			return transform{}, ErrGeometry
		}

		output.world[index] = float64(value)
	}

	if input[3] != 0 || input[7] != 0 || input[11] != 0 || input[15] != 1 {
		return transform{}, ErrGeometry
	}

	a, b, c := output.world[0], output.world[4], output.world[8]
	d, e, f := output.world[1], output.world[5], output.world[9]
	g, h, i := output.world[2], output.world[6], output.world[10]

	determinant := a*(e*i-f*h) - b*(d*i-f*g) + c*(d*h-e*g)
	if !finite(determinant) || determinant <= 0 {
		return transform{}, ErrGeometry
	}

	// Row-major inverse transpose, preserving normals under nonuniform scale.
	output.normal = [9]float64{(e*i - f*h) / determinant, (f*g - d*i) / determinant, (d*h - e*g) / determinant,
		(c*h - b*i) / determinant, (a*i - c*g) / determinant, (b*g - a*h) / determinant,
		(b*f - c*e) / determinant, (c*d - a*f) / determinant, (a*e - b*d) / determinant}
	for _, value := range output.normal {
		if !finite(value) {
			return transform{}, ErrGeometry
		}
	}

	return output, nil
}

func (value transform) vertex(data []byte) (vertex, error) {
	var input [8]float64
	for index := range input {
		input[index] = float64(math.Float32frombits(binary.LittleEndian.Uint32(data[index*4:])))
	}

	result := vertex{uv: [2]float64{input[6], input[7]}}
	for axis := range 3 {
		position := value.world[axis]*input[0] + value.world[4+axis]*input[1] + value.world[8+axis]*input[2] + value.world[12+axis]
		if !finite(position) || math.Abs(position) > math.MaxFloat32 {
			return vertex{}, ErrGeometry
		}

		// Match the prepared occluder representation before clipping boundaries.
		result.position[axis] = float64(float32(position))
		result.normal[axis] = value.normal[axis*3]*input[3] + value.normal[axis*3+1]*input[4] + value.normal[axis*3+2]*input[5]
	}

	result.normal = unit(result.normal)

	return result, nil
}

func unit(value vector) vector {
	length := math.Sqrt(value[0]*value[0] + value[1]*value[1] + value[2]*value[2])
	if length == 0 {
		return vector{}
	}

	return vector{value[0] / length, value[1] / length, value[2] / length}
}

func areaSquared(value triangle) float64 {
	a, b := vector{}, vector{}
	for axis := range 3 {
		a[axis] = value[1].position[axis] - value[0].position[axis]
		b[axis] = value[2].position[axis] - value[0].position[axis]
	}

	x, y, z := a[1]*b[2]-a[2]*b[1], a[2]*b[0]-a[0]*b[2], a[0]*b[1]-a[1]*b[0]

	return x*x + y*y + z*z
}
