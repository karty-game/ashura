//go:build !js

//nolint:mnd,varnamelen // Ray/BVH geometry uses standard slab and barycentric equations.
package lightmapbuild

import (
	"encoding/binary"
	"math"
)

type transportTriangle struct {
	origin, first, second, normal vec
	surface                       [4]float32
}
type transportScene struct {
	nodes     []bvhNode
	triangles []transportTriangle
	ordered   bool
}

func newTransportScene(nodes []byte, triangles []triangle, surfaces []byte) transportScene {
	scene := transportScene{nodes: make([]bvhNode, len(nodes)/32), triangles: make([]transportTriangle, len(triangles))}
	for i := range scene.nodes {
		data := nodes[i*32:]
		scene.nodes[i] = bvhNode{
			minimum: vec{readFloat(data), readFloat(data[4:]), readFloat(data[8:])},
			maximum: vec{readFloat(data[16:]), readFloat(data[20:]), readFloat(data[24:])},
			first:   binary.LittleEndian.Uint32(data[12:]),
			count:   binary.LittleEndian.Uint32(data[28:]),
		}
	}

	for i, triangle := range triangles {
		first, second := sub(triangle.points[1], triangle.points[0]), sub(triangle.points[2], triangle.points[0])
		scene.triangles[i] = transportTriangle{
			origin: triangle.points[0],
			first:  first,
			second: second,
			normal: normalize(cross(first, second)),
			surface: [4]float32{
				readFloat(surfaces[i*16:]),
				readFloat(surfaces[i*16+4:]),
				readFloat(surfaces[i*16+8:]),
				readFloat(surfaces[i*16+12:]),
			},
		}
	}

	return scene
}

type transportEntry struct {
	index    uint32
	distance float32
}

func (scene transportScene) trace(origin, direction vec) (int, float32) {
	if scene.ordered {
		return scene.traceNearFirst(origin, direction)
	}

	return scene.traceUnordered(origin, direction)
}

func (scene transportScene) traceNearFirst(origin, direction vec) (int, float32) {
	inverse := vec{1 / direction[0], 1 / direction[1], 1 / direction[2]}
	nearest := float32(math.Inf(1))

	entry, hit := transportBoxEntry(scene.nodes[0].bounds, origin, inverse, nearest)
	if !hit {
		return -1, nearest
	}

	stack := [64]transportEntry{}
	stack[0] = transportEntry{index: 0, distance: entry}
	top, found := 1, -1

	var selectedLeaf uint32

	for top > 0 {
		top--

		current := stack[top]
		if current.distance > nearest {
			continue
		}

		node := scene.nodes[current.index]
		if node.count == 0 {
			top = scene.pushChildren(&stack, top, node.first, origin, inverse, nearest)

			continue
		}

		for index := node.first; index < node.first+node.count; index++ {
			distance := transportIntersection(scene.triangles[index], origin, direction)
			// Preserve the old traversal's tie winner: descending leaf ranges, then
			// ascending triangle order inside a leaf. This makes near-first ordering
			// numerically compatible even for coincident surfaces/material boundaries.
			tie := found >= 0 && distance == nearest && (node.first > selectedLeaf || node.first == selectedLeaf && int(index) < found)
			if distance > 1e-4 && (distance < nearest || tie) {
				nearest, found, selectedLeaf = distance, int(index), node.first
			}
		}
	}

	return found, nearest
}

func (scene transportScene) pushChildren(stack *[64]transportEntry, top int, first uint32, origin, inverse vec, nearest float32) int {
	left, hitLeft := transportBoxEntry(scene.nodes[first].bounds, origin, inverse, nearest)

	right, hitRight := transportBoxEntry(scene.nodes[first+1].bounds, origin, inverse, nearest)
	if hitLeft && hitRight {
		near, far := transportEntry{index: first, distance: left}, transportEntry{index: first + 1, distance: right}
		if right <= left {
			near, far = far, near
		}

		stack[top], stack[top+1] = far, near

		return top + 2
	}

	if hitLeft {
		stack[top] = transportEntry{index: first, distance: left}

		return top + 1
	}

	if hitRight {
		stack[top] = transportEntry{index: first + 1, distance: right}

		return top + 1
	}

	return top
}

func (scene transportScene) traceUnordered(origin, direction vec) (int, float32) {
	inverse := vec{1 / direction[0], 1 / direction[1], 1 / direction[2]}
	stack := [64]uint32{}
	top := 1
	nearest := float32(math.Inf(1))
	found := -1

	for top > 0 {
		top--

		node := scene.nodes[stack[top]]
		if !transportBox(node.bounds, origin, inverse, nearest) {
			continue
		}

		if node.count == 0 {
			stack[top], stack[top+1] = node.first, node.first+1
			top += 2

			continue
		}

		for index := node.first; index < node.first+node.count; index++ {
			distance := transportIntersection(scene.triangles[index], origin, direction)
			if distance > 1e-4 && distance < nearest {
				nearest, found = distance, int(index)
			}
		}
	}

	return found, nearest
}

func transportBox(box bounds, origin, inverse vec, limit float32) bool {
	low, high := float32(0), limit

	for axis := range 3 {
		if inverse[axis] > math.MaxFloat32 || inverse[axis] < -math.MaxFloat32 {
			if origin[axis] < box.minimum[axis] || origin[axis] > box.maximum[axis] {
				return false
			}

			continue
		}

		a, b := (box.minimum[axis]-origin[axis])*inverse[axis], (box.maximum[axis]-origin[axis])*inverse[axis]
		low = max(low, min(a, b))
		high = min(high, max(a, b))
	}

	return high >= low
}

func transportBoxEntry(box bounds, origin, inverse vec, limit float32) (float32, bool) {
	low, high := float32(0), limit

	for axis := range 3 {
		if inverse[axis] > math.MaxFloat32 || inverse[axis] < -math.MaxFloat32 {
			if origin[axis] < box.minimum[axis] || origin[axis] > box.maximum[axis] {
				return 0, false
			}

			continue
		}

		a, b := (box.minimum[axis]-origin[axis])*inverse[axis], (box.maximum[axis]-origin[axis])*inverse[axis]
		low = max(low, min(a, b))

		high = min(high, max(a, b))
		if high < low {
			return 0, false
		}
	}

	return low, high >= low
}

func transportIntersection(triangle transportTriangle, origin, direction vec) float32 {
	perpendicular := cross(direction, triangle.second)

	determinant := dot(triangle.first, perpendicular)
	if math.Abs(float64(determinant)) < 1e-9 {
		return -1
	}

	delta := sub(origin, triangle.origin)
	inverse := 1 / determinant

	first := dot(delta, perpendicular) * inverse
	if first < -1e-6 || first > 1.000001 {
		return -1
	}

	perpendicular = cross(delta, triangle.first)

	second := dot(direction, perpendicular) * inverse
	if second < -1e-6 || first+second > 1.000001 {
		return -1
	}

	return dot(triangle.second, perpendicular) * inverse
}
