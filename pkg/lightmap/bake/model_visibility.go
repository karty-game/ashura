//go:build !js

package lightmapbake

import "math/big"

// hasWorldArea removes only exact zero-measure visibility geometry. Thin
// positive-area triangles survive; the solver admits its numeric bounds.
func hasWorldArea(triangle Triangle) bool {
	var first, second [3]float64
	for axis := range 3 {
		first[axis] = float64(triangle.Positions[1][axis]) - float64(triangle.Positions[0][axis])
		second[axis] = float64(triangle.Positions[2][axis]) - float64(triangle.Positions[0][axis])
	}

	if first[1]*second[2]-first[2]*second[1] != 0 || first[2]*second[0]-first[0]*second[2] != 0 ||
		first[0]*second[1]-first[1]*second[0] != 0 {
		return true
	}
	// Only a zero candidate needs exact arithmetic. Widely separated float32
	// coordinates can lose positive support even in float64 subtraction.
	return exactWorldArea(triangle)
}

func exactWorldArea(triangle Triangle) bool {
	var first, second [3]big.Rat

	for axis := range 3 {
		var origin, corner big.Rat
		origin.SetFloat64(float64(triangle.Positions[0][axis]))
		corner.SetFloat64(float64(triangle.Positions[1][axis]))
		first[axis].Sub(&corner, &origin)
		corner.SetFloat64(float64(triangle.Positions[2][axis]))
		second[axis].Sub(&corner, &origin)
	}

	for axis := range 3 {
		next := (axis + 1) % len(first)
		last := (next + 1) % len(first)

		var left, right big.Rat
		left.Mul(&first[next], &second[last])
		right.Mul(&first[last], &second[next])

		if left.Cmp(&right) != 0 {
			return true
		}
	}

	return false
}
