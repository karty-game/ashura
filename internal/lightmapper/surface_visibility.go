package lightmapper

import "math"

// Surface support casts many short any-hit rays against the same immutable
// scene. Store only the intersection inputs, in BVH leaf order, rather than
// repeatedly reading full material/shading triangles and rebuilding their edges.
type surfaceVisibility struct {
	nodes     []BVHNode
	triangles []surfaceVisibilityTriangle
	bounds    []surfaceVisibilityBounds
}

type surfaceVisibilityBounds struct {
	low, high Vec3
}

func (bounds surfaceVisibilityBounds) overlaps(low, high Vec3) bool {
	return !(bounds.low[0] > high[0] || bounds.high[0] < low[0] ||
		bounds.low[1] > high[1] || bounds.high[1] < low[1] ||
		bounds.low[2] > high[2] || bounds.high[2] < low[2])
}

type surfaceVisibilityTriangle struct {
	origin, first, second Vec3
	area                  float32
}

func newSurfaceVisibility(scene *PreparedScene) surfaceVisibility {
	result := surfaceVisibility{
		nodes: scene.nodes, triangles: make([]surfaceVisibilityTriangle, len(scene.order)),
		bounds: make([]surfaceVisibilityBounds, len(scene.order)),
	}
	for index, source := range scene.order {
		triangle := scene.source.Triangles[source]
		first := Sub(triangle.Positions[1], triangle.Positions[0])
		second := Sub(triangle.Positions[2], triangle.Positions[0])
		result.triangles[index] = surfaceVisibilityTriangle{
			origin: triangle.Positions[0], first: first, second: second, area: Length(Cross(first, second)),
		}

		low := minVec(triangle.Positions[0], minVec(triangle.Positions[1], triangle.Positions[2]))
		high := maxVec(triangle.Positions[0], maxVec(triangle.Positions[1], triangle.Positions[2]))

		margin := surfaceTolerance(low, high, 0)
		for axis := range 3 {
			low[axis] -= margin
			high[axis] += margin
		}

		result.bounds[index] = surfaceVisibilityBounds{low: low, high: high}
	}

	return result
}

func (visibility surfaceVisibility) visible(first, second Receiver, explicit float32) bool {
	ray := surfaceVisibilityRay(first, second, explicit)

	return ray.Max == 0 || !visibility.occluded(ray)
}

func surfaceVisibilityRay(first, second Receiver, explicit float32) Ray {
	return surfaceGuideVisibilityRay(first.Position, second.Position, first.GeometricNormal, second.GeometricNormal, explicit)
}

func surfaceGuideVisibilityRay(first, second, firstNormal, secondNormal Vec3, explicit float32) Ray {
	tolerance := surfaceTolerance(first, second, explicit)
	origin := Add(first, Scale(firstNormal, tolerance))
	destination := Add(second, Scale(secondNormal, tolerance))
	delta := Sub(destination, origin)

	distance := Length(delta)
	if distance <= 2*tolerance {
		return Ray{}
	}

	return Ray{
		Origin: origin, Direction: Scale(delta, 1/distance), Min: tolerance, Max: distance - tolerance,
	}
}

// A support owner's segments share a small spatial neighborhood. Query that
// neighborhood once and retain every overlapping triangle. Original triangle
// bounds include coordinate-precision margins; loose sibling bounds in a leaf
// do not enlarge the candidate set. A large result selects the full BVH traversal;
// this threshold never truncates occluders or changes the visibility scene.
func (visibility surfaceVisibility) candidates(low, high Vec3, output []uint32) ([]uint32, bool) {
	output = output[:0]
	if !validVec(low) || !validVec(high) {
		return output, false
	}

	if len(visibility.nodes) == 0 {
		return output, true
	}

	margin := surfaceTolerance(low, high, 0)
	for axis := range 3 {
		low[axis] -= margin
		high[axis] += margin
	}

	var stack [64]uint32

	size := 1
	for size > 0 {
		size--

		node := visibility.nodes[stack[size]]
		if node.Min[0] > high[0] || node.Max[0] < low[0] ||
			node.Min[1] > high[1] || node.Max[1] < low[1] ||
			node.Min[2] > high[2] || node.Max[2] < low[2] {
			continue
		}

		if node.Count == 0 {
			stack[size], stack[size+1] = node.Left, node.Right
			size += 2

			continue
		}

		for index := node.First; index < node.First+node.Count; index++ {
			if !visibility.bounds[index].overlaps(low, high) {
				continue
			}

			const maximumLocalTriangles = 128
			if len(output) == maximumLocalTriangles {
				return output[:0], false
			}

			output = append(output, index)
		}
	}

	return output, true
}

func (visibility surfaceVisibility) occludedCandidates(ray Ray, candidates []uint32) bool {
	threshold := float32(1e-7) * Length(ray.Direction)
	for _, index := range candidates {
		if visibility.triangles[index].occluded(ray, threshold) {
			return true
		}
	}

	return false
}

// The ray interval and triangle acceptance threshold match PreparedScene's
// reference intersection. Reciprocal directions are shared by all visited boxes;
// near-zero axes retain the reference's explicit parallel-slab handling.
func (visibility surfaceVisibility) occluded(ray Ray) bool {
	if len(visibility.nodes) == 0 {
		return false
	}

	var inverse Vec3

	var parallel [3]bool

	for axis := range 3 {
		const parallelThreshold = 1e-20

		parallel[axis] = float32(math.Abs(float64(ray.Direction[axis]))) < parallelThreshold
		if !parallel[axis] {
			inverse[axis] = 1 / ray.Direction[axis]
		}
	}

	threshold := float32(1e-7) * Length(ray.Direction)

	var stack [64]uint32

	size := 1
	for size > 0 {
		size--

		node := visibility.nodes[stack[size]]
		if !surfaceVisibilityBox(ray, inverse, parallel, node.Min, node.Max) {
			continue
		}

		if node.Count == 0 {
			stack[size], stack[size+1] = node.Left, node.Right
			size += 2

			continue
		}

		for _, triangle := range visibility.triangles[node.First : node.First+node.Count] {
			if triangle.occluded(ray, threshold) {
				return true
			}
		}
	}

	return false
}

func surfaceVisibilityBox(ray Ray, inverse Vec3, parallel [3]bool, low, high Vec3) bool {
	near, far := ray.Min, ray.Max

	for axis := range 3 {
		if parallel[axis] {
			if ray.Origin[axis] < low[axis] || ray.Origin[axis] > high[axis] {
				return false
			}

			continue
		}

		first := (low[axis] - ray.Origin[axis]) * inverse[axis]
		second := (high[axis] - ray.Origin[axis]) * inverse[axis]
		near = max(near, min(first, second))

		far = min(far, max(first, second))
		if near > far {
			return false
		}
	}

	return true
}

func (triangle surfaceVisibilityTriangle) occluded(ray Ray, threshold float32) bool {
	cross := Cross(ray.Direction, triangle.second)

	determinant := Dot(triangle.first, cross)
	if float32(math.Abs(float64(determinant))) <= threshold*triangle.area {
		return false
	}

	delta := Sub(ray.Origin, triangle.origin)

	first := Dot(delta, cross) / determinant
	if first < 0 || first > 1 {
		return false
	}

	cross = Cross(delta, triangle.first)

	second := Dot(ray.Direction, cross) / determinant
	if second < 0 || first+second > 1 {
		return false
	}

	distance := Dot(triangle.second, cross) / determinant

	return distance >= ray.Min && distance < ray.Max
}
