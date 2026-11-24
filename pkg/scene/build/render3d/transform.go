//go:build !js

package render3dbuild

import (
	"errors"
	"fmt"
	"math"

	scenebuild "github.com/karty-game/ashura/pkg/scene/build"
)

type matrix [16]float64

var (
	errScale       = errors.New("authored scale must be positive")
	errSingular    = errors.New("non-finite or singular composed transform")
	errShear       = errors.New("composed transform contains shear; bake it into a prepared model")
	errFloatBounds = errors.New("composed transform is outside finite float32 bounds")
)

func vector(config map[string]any, property string, fallback [3]float64) [3]float64 {
	values, ok := config[property].([]any)
	if !ok {
		return fallback
	}

	for index := range fallback {
		fallback[index], _ = values[index].(float64)
	}

	return fallback
}

func vectorCode(value [3]float64) string {
	return fmt.Sprintf("engine.Vector3{X:%.9g,Y:%.9g,Z:%.9g}", value[0], value[1], value[2])
}

func transformCode(component *scenebuild.Component) (string, error) {
	world, err := composedTransform(component.Ancestors, component.NodeComponents())
	if err != nil {
		return "", fmt.Errorf("%s: %w", component.Path, err)
	}

	position, rotation, scale, err := decompose(world)
	if err != nil {
		return "", fmt.Errorf("%s: %w", component.Path, err)
	}

	return fmt.Sprintf(
		"engine.Transform{Position:%s,Rotation:%s,Scale:%s}",
		vectorCode(position),
		vectorCode(rotation),
		vectorCode(scale),
	), nil
}

// WorldTransform composes the same static transform used by generated factories.
// The result is column-major and admits only finite, nonsingular, shear-free TRS.
func WorldTransform(ancestors []map[string]map[string]any, components map[string]map[string]any) ([16]float32, error) {
	world, err := composedTransform(ancestors, components)
	if err != nil {
		return [16]float32{}, err
	}

	if _, _, _, err = decompose(world); err != nil {
		return [16]float32{}, err
	}

	var result [16]float32
	for index, value := range world {
		result[index] = float32(value)
	}

	return result, nil
}

func composedTransform(ancestors []map[string]map[string]any, node map[string]map[string]any) (matrix, error) {
	world := matrix{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}

	configs := append(append([]map[string]map[string]any(nil), ancestors...), node)
	for _, components := range configs {
		config := components["spatial.transform3d"]
		position := vector(config, "position", [3]float64{})
		rotation := vector(config, "rotation", [3]float64{})

		scale := vector(config, "scale", [3]float64{1, 1, 1})
		for _, value := range scale {
			if value <= 0 {
				return matrix{}, errScale
			}
		}

		world = multiply(world, local(position, rotation, scale))
	}

	return world, nil
}

func local(position, rotation, scale [3]float64) matrix {
	sinX, cosX := math.Sincos(rotation[0])
	sinY, cosY := math.Sincos(rotation[1])
	sinZ, cosZ := math.Sincos(rotation[2])

	return matrix{
		cosZ * cosY * scale[0], sinZ * cosY * scale[0], -sinY * scale[0], 0,
		(cosZ*sinY*sinX - sinZ*cosX) * scale[1], (sinZ*sinY*sinX + cosZ*cosX) * scale[1], cosY * sinX * scale[1], 0,
		(cosZ*sinY*cosX + sinZ*sinX) * scale[2], (sinZ*sinY*cosX - cosZ*sinX) * scale[2], cosY * cosX * scale[2], 0,
		position[0], position[1], position[2], 1,
	}
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

func decompose(value matrix) (position, rotation, scale [3]float64, err error) {
	position = [3]float64{value[12], value[13], value[14]}
	for column := range 3 {
		scale[column] = math.Sqrt(
			value[column*4]*value[column*4] + value[column*4+1]*value[column*4+1] + value[column*4+2]*value[column*4+2],
		)
		if scale[column] <= 0 || math.IsNaN(scale[column]) || math.IsInf(scale[column], 0) {
			return position, rotation, scale, errSingular
		}

		for row := range 3 {
			value[column*4+row] /= scale[column]
		}
	}

	const tolerance = 1e-6

	for first := range 3 {
		for second := first + 1; second < 3; second++ {
			dot := float64(0)
			for row := range 3 {
				dot += value[first*4+row] * value[second*4+row]
			}

			if math.Abs(dot) > tolerance {
				return position, rotation, scale, errShear
			}
		}
	}

	rotation[1] = math.Asin(math.Max(-1, math.Min(1, -value[2])))
	if math.Abs(math.Cos(rotation[1])) > tolerance {
		rotation[0], rotation[2] = math.Atan2(value[6], value[10]), math.Atan2(value[1], value[0])
	} else {
		rotation[0], rotation[2] = math.Atan2(-value[9], value[5]), 0
	}

	for _, vector := range [][3]float64{position, rotation, scale} {
		for _, component := range vector {
			if math.IsNaN(component) || math.IsInf(component, 0) || math.Abs(component) > math.MaxFloat32 {
				return position, rotation, scale, errFloatBounds
			}
		}
	}

	return position, rotation, scale, nil
}
