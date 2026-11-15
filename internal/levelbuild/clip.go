//go:build !js

//nolint:mnd // A clipped polygon must have at least three corners to contain triangles.
package levelbuild

import "math"

func cells(value triangle, options Options) (minimum, maximum [3]int32, err error) {
	count := uint64(1)

	for axis := range 3 {
		low := min(value[0].position[axis], value[1].position[axis], value[2].position[axis])
		high := max(value[0].position[axis], value[1].position[axis], value[2].position[axis])
		origin, size := float64(options.Origin[axis]), float64(options.Size[axis])
		first := math.Floor((low - origin) / size)

		last := math.Ceil((high-origin)/size) - 1
		if high == low {
			last = first
		}

		if !finite(first) || !finite(last) || first < math.MinInt32 || last > math.MaxInt32 || last < first {
			return minimum, maximum, ErrGeometry
		}

		count *= uint64(last-first) + 1
		if count > uint64(options.MaxChunks) {
			return minimum, maximum, ErrGeometry
		}

		minimum[axis], maximum[axis] = int32(first), int32(last)
	}

	return minimum, maximum, nil
}

func cellBounds(cell [3]int32, options Options) (minimum, maximum vector) {
	for axis := range 3 {
		origin, size := float64(options.Origin[axis]), float64(options.Size[axis])
		// Adjacent cells use the same representable GPU boundary, including
		// non-power-of-two cell sizes such as 0.1 units.
		minimum[axis] = float64(float32(origin + float64(cell[axis])*size))
		maximum[axis] = float64(float32(origin + (float64(cell[axis])+1)*size))
	}

	return minimum, maximum
}

func clip(value triangle, minimum, maximum vector) []vertex {
	polygon := []vertex{value[0], value[1], value[2]}
	for axis := range 3 {
		polygon = clipPlane(polygon, axis, minimum[axis], true)

		polygon = clipPlane(polygon, axis, maximum[axis], false)
		if len(polygon) < 3 {
			return nil
		}
	}

	return polygon
}

func clipPlane(input []vertex, axis int, boundary float64, greater bool) []vertex {
	if len(input) == 0 {
		return nil
	}

	output := make([]vertex, 0, len(input)+1)
	previous := input[len(input)-1]

	previousInside := inside(previous.position[axis], boundary, greater)
	for _, current := range input {
		currentInside := inside(current.position[axis], boundary, greater)
		if currentInside != previousInside {
			output = append(output, intersection(previous, current, axis, boundary))
		}

		if currentInside {
			output = append(output, current)
		}

		previous, previousInside = current, currentInside
	}

	return output
}

func inside(value, boundary float64, greater bool) bool {
	if greater {
		return value >= boundary
	}

	return value <= boundary
}

func intersection(first, second vertex, axis int, boundary float64) vertex {
	// Canonical edge orientation makes adjacent cells compute shared cuts in
	// the same order, including when their polygons traverse an edge backwards.
	for coordinate := range 3 {
		if first.position[coordinate] == second.position[coordinate] {
			continue
		}

		if first.position[coordinate] > second.position[coordinate] {
			first, second = second, first
		}

		break
	}

	amount := (boundary - first.position[axis]) / (second.position[axis] - first.position[axis])

	var result vertex
	for coordinate := range 3 {
		result.position[coordinate] = first.position[coordinate] + amount*(second.position[coordinate]-first.position[coordinate])
		result.normal[coordinate] = first.normal[coordinate] + amount*(second.normal[coordinate]-first.normal[coordinate])
	}

	for coordinate := range 2 {
		result.uv[coordinate] = first.uv[coordinate] + amount*(second.uv[coordinate]-first.uv[coordinate])
	}

	result.position[axis] = boundary

	return result
}
