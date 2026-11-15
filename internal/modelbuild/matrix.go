//go:build !js

//nolint:mnd // Fixed glTF column-major matrix, quaternion and vector component indices.
package modelbuild

import "math"

type matrix [16]float64
type vector [3]float64

func identityMatrix() matrix { return matrix{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1} }

func localMatrix(value node) (matrix, error) {
	if value.Matrix != nil {
		if len(value.Matrix) != 16 || value.Translation != nil || value.Rotation != nil || value.Scale != nil {
			return matrix{}, errFormat
		}

		var result matrix
		copy(result[:], value.Matrix)

		return checkedMatrix(result)
	}

	translation, scale := vector{}, vector{1, 1, 1}

	if value.Translation != nil {
		if len(value.Translation) != 3 {
			return matrix{}, errFormat
		}

		copy(translation[:], value.Translation)
	}

	if value.Scale != nil {
		if len(value.Scale) != 3 {
			return matrix{}, errFormat
		}

		copy(scale[:], value.Scale)
	}

	rotation := [4]float64{0, 0, 0, 1}

	if value.Rotation != nil {
		if len(value.Rotation) != 4 {
			return matrix{}, errFormat
		}

		copy(rotation[:], value.Rotation)
	}

	quatX, quatY, quatZ, scalar := rotation[0], rotation[1], rotation[2], rotation[3]

	length := quatX*quatX + quatY*quatY + quatZ*quatZ + scalar*scalar
	if !finite(length) || math.Abs(length-1) > .0001 {
		return matrix{}, errFormat
	}

	result := matrix{
		(1 - 2*(quatY*quatY+quatZ*quatZ)) * scale[0],
		2 * (quatX*quatY + quatZ*scalar) * scale[0],
		2 * (quatX*quatZ - quatY*scalar) * scale[0],
		0,
		2 * (quatX*quatY - quatZ*scalar) * scale[1],
		(1 - 2*(quatX*quatX+quatZ*quatZ)) * scale[1],
		2 * (quatY*quatZ + quatX*scalar) * scale[1],
		0,
		2 * (quatX*quatZ + quatY*scalar) * scale[2],
		2 * (quatY*quatZ - quatX*scalar) * scale[2],
		(1 - 2*(quatX*quatX+quatY*quatY)) * scale[2],
		0,
		translation[0],
		translation[1],
		translation[2],
		1,
	}

	return checkedMatrix(result)
}

func checkedMatrix(value matrix) (matrix, error) {
	for _, component := range value {
		if !finite(component) {
			return matrix{}, errFormat
		}
	}

	if value[3] != 0 || value[7] != 0 || value[11] != 0 || value[15] != 1 || determinant(value) == 0 || !finite(determinant(value)) {
		return matrix{}, errFormat
	}

	return value, nil
}

func multiply(first, second matrix) matrix {
	var result matrix

	for column := range 4 {
		for row := range 4 {
			for shared := range 4 {
				result[column*4+row] += first[shared*4+row] * second[column*4+shared]
			}
		}
	}

	return result
}

func position(value matrix, point vector) vector {
	return vector{value[0]*point[0] + value[4]*point[1] + value[8]*point[2] + value[12],
		value[1]*point[0] + value[5]*point[1] + value[9]*point[2] + value[13],
		value[2]*point[0] + value[6]*point[1] + value[10]*point[2] + value[14]}
}

func determinant(value matrix) float64 {
	return value[0]*(value[5]*value[10]-value[9]*value[6]) - value[4]*(value[1]*value[10]-value[9]*value[2]) + value[8]*(value[1]*value[6]-value[5]*value[2])
}

func normal(value matrix, direction vector) vector {
	first, second, third := vector{
		value[0],
		value[1],
		value[2],
	}, vector{
		value[4],
		value[5],
		value[6],
	}, vector{
		value[8],
		value[9],
		value[10],
	}
	cofactor := [3]vector{cross(second, third), cross(third, first), cross(first, second)}

	var result vector

	for axis := range 3 {
		for component := range 3 {
			result[axis] += cofactor[component][axis] * direction[component] / determinant(value)
		}
	}

	return normalized(result)
}

func cross(first, second vector) vector {
	return vector{first[1]*second[2] - first[2]*second[1], first[2]*second[0] - first[0]*second[2], first[0]*second[1] - first[1]*second[0]}
}

func subtract(first, second vector) vector {
	return vector{first[0] - second[0], first[1] - second[1], first[2] - second[2]}
}

func normalized(value vector) vector {
	length := math.Sqrt(value[0]*value[0] + value[1]*value[1] + value[2]*value[2])
	if length == 0 {
		return vector{0, 1, 0}
	}

	return vector{value[0] / length, value[1] / length, value[2] / length}
}

func finite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }
