//go:build !js

package impostorbake

import "math"

const (
	srgbToLinearThreshold = .04045
	linearToSRGBThreshold = .0031308
	srgbScale             = 12.92
	srgbOffset            = .055
	srgbRange             = 1.055
	srgbPower             = 2.4
)

type vector [3]float64

func add(left, right vector) vector {
	return vector{left[0] + right[0], left[1] + right[1], left[2] + right[2]}
}
func subtract(left, right vector) vector {
	return vector{left[0] - right[0], left[1] - right[1], left[2] - right[2]}
}
func scale(value vector, factor float64) vector {
	return vector{value[0] * factor, value[1] * factor, value[2] * factor}
}
func dot(left, right vector) float64 { return left[0]*right[0] + left[1]*right[1] + left[2]*right[2] }
func cross(left, right vector) vector {
	return vector{left[1]*right[2] - left[2]*right[1], left[2]*right[0] - left[0]*right[2], left[0]*right[1] - left[1]*right[0]}
}
func unit(value vector) vector {
	length := math.Sqrt(dot(value, value))
	if length < geometryEpsilon {
		return vector{0, 0, 1}
	}

	return scale(value, 1/length)
}
func floatVector(value vector) [3]float32 {
	return [3]float32{float32(value[0]), float32(value[1]), float32(value[2])}
}
func doubleVector(value [3]float32) vector {
	return vector{float64(value[0]), float64(value[1]), float64(value[2])}
}
func finite(value float64) bool   { return !math.IsNaN(value) && !math.IsInf(value, 0) }
func quantize(value float64) byte { return byte(math.Round(min(1, max(0, value)) * byteMaximum)) }
func toLinear(value float64) float64 {
	if value <= srgbToLinearThreshold {
		return value / srgbScale
	}

	return math.Pow((value+srgbOffset)/srgbRange, srgbPower)
}
func toSRGB(value float64) float64 {
	if value <= linearToSRGBThreshold {
		return value * srgbScale
	}

	return srgbRange*math.Pow(value, 1/srgbPower) - srgbOffset
}
