package lightmapper

import "math"

// receiverPositiveUVOverlap checks original float32 UV geometry through float64
// clipping, before quantizing stored barycentrics. Reconstructing the polygon
// from float32 barycentrics could falsely overlap valid adjoining triangles.
//
//nolint:mnd // A polygon needs three corners; 128 float64 coordinate ULPs bound clipping roundoff before admitting positive area.
func receiverPositiveUVOverlap(polygon []receiverClipVertex, uv [3]Vec2, width, height uint32) bool {
	triangle := [3][2]float64{}
	for i := range 3 {
		triangle[i] = [2]float64{float64(uv[i][0]) * float64(width), float64(uv[i][1]) * float64(height)}
	}

	orientation := receiverUVSide(triangle[0], triangle[1], triangle[2])
	if orientation == 0 {
		return false
	}

	clipped := polygon
	for edge := range 3 {
		clipped = receiverClipUVEdge(clipped, triangle[edge], triangle[(edge+1)%3], orientation > 0)
		if len(clipped) < 3 {
			return false
		}
	}

	magnitude := float64(0)
	for _, vertex := range clipped {
		magnitude = max(magnitude, math.Abs(vertex.uv[0]), math.Abs(vertex.uv[1]))
	}

	tolerance := 128 * (math.Nextafter(magnitude, math.Inf(1)) - magnitude)

	return receiverPolygonArea(clipped) > tolerance
}

func receiverUVSide(a, b, p [2]float64) float64 {
	return (b[0]-a[0])*(p[1]-a[1]) - (b[1]-a[1])*(p[0]-a[0])
}

//nolint:varnamelen // a/b are the oriented clip-edge endpoints.
func receiverClipUVEdge(input []receiverClipVertex, a, b [2]float64, positive bool) []receiverClipVertex {
	if len(input) == 0 {
		return nil
	}

	result := make([]receiverClipVertex, 0, len(input)+1)
	previous := input[len(input)-1]

	last := receiverUVSide(a, b, previous.uv)
	if !positive {
		last = -last
	}

	for _, current := range input {
		next := receiverUVSide(a, b, current.uv)
		if !positive {
			next = -next
		}

		if (last >= 0) != (next >= 0) {
			t := last / (last - next)

			var intersection receiverClipVertex
			for axis := range 2 {
				intersection.uv[axis] = previous.uv[axis] + t*(current.uv[axis]-previous.uv[axis])
			}

			result = append(result, intersection)
		}

		if next >= 0 {
			result = append(result, current)
		}

		previous, last = current, next
	}

	return result
}
