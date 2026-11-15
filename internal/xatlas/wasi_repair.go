//go:build !js

//nolint:mnd,gosec // Bounded wasm32 UV-mesh fields and source vertex coordinates.
package xatlas

import (
	"context"
	"encoding/binary"
	"math"
	"sort"
)

type uvPoint struct{ x, y float64 }
type uvTriangle struct {
	points    [3]uvPoint
	low, high uvPoint
}
type uvNode struct {
	low, high   uvPoint
	triangles   []uvTriangle
	left, right *uvNode
}

func uvSide(a, b, corner uvPoint) float64 { return (b.x-a.x)*(corner.y-a.y) - (b.y-a.y)*(corner.x-a.x) }

func uvIntersects(subject, clip uvTriangle) bool {
	var first, second [12]uvPoint
	copy(first[:], subject.points[:])

	count := 3

	orientation := uvSide(clip.points[0], clip.points[1], clip.points[2])
	if orientation == 0 {
		return false
	}

	sign := 1.0
	if orientation < 0 {
		sign = -1
	}

	for edge := 0; edge < 3 && count >= 3; edge++ {
		next := 0
		previous := first[count-1]

		last := uvSide(clip.points[edge], clip.points[(edge+1)%3], previous) * sign
		for _, current := range first[:count] {
			value := uvSide(clip.points[edge], clip.points[(edge+1)%3], current) * sign
			if (last >= 0) != (value >= 0) {
				ratio := last / (last - value)
				second[next] = uvPoint{previous.x + ratio*(current.x-previous.x), previous.y + ratio*(current.y-previous.y)}
				next++
			}

			if value >= 0 {
				second[next] = current
				next++
			}

			previous, last = current, value
		}

		count = next
		copy(first[:], second[:count])
	}

	if count < 3 {
		return false
	}

	area, magnitude := 0.0, 0.0
	for _, part := range first[:count] {
		magnitude = max(magnitude, math.Abs(part.x), math.Abs(part.y))
	}

	for i := 1; i+1 < count; i++ {
		area += uvSide(first[0], first[i], first[i+1])
	}

	return math.Abs(area)*.5 > 128*(math.Nextafter(magnitude, math.Inf(1))-magnitude)
}

func uvTree(ctx context.Context, triangles []uvTriangle) *uvNode {
	checkRepairContext(ctx)

	node := &uvNode{low: uvPoint{math.Inf(1), math.Inf(1)}, high: uvPoint{math.Inf(-1), math.Inf(-1)}}

	for index, triangle := range triangles {
		if index%256 == 0 {
			checkRepairContext(ctx)
		}

		node.low.x = min(node.low.x, triangle.low.x)
		node.low.y = min(node.low.y, triangle.low.y)
		node.high.x = max(node.high.x, triangle.high.x)
		node.high.y = max(node.high.y, triangle.high.y)
	}

	if len(triangles) <= 8 {
		node.triangles = triangles

		return node
	}

	splitX := node.high.x-node.low.x > node.high.y-node.low.y

	comparisons := 0

	sort.Slice(triangles, func(leftIndex, rightIndex int) bool {
		comparisons++
		if comparisons%1024 == 0 {
			checkRepairContext(ctx)
		}

		if splitX {
			return triangles[leftIndex].low.x+triangles[leftIndex].high.x < triangles[rightIndex].low.x+triangles[rightIndex].high.x
		}

		return triangles[leftIndex].low.y+triangles[leftIndex].high.y < triangles[rightIndex].low.y+triangles[rightIndex].high.y
	})
	middle := len(triangles) / 2
	node.left, node.right = uvTree(ctx, triangles[:middle]), uvTree(ctx, triangles[middle:])

	return node
}
func (node *uvNode) overlaps(ctx context.Context, triangle uvTriangle, identity *uvTriangle) bool {
	checkRepairContext(ctx)

	if triangle.high.x <= node.low.x || triangle.low.x >= node.high.x || triangle.high.y <= node.low.y || triangle.low.y >= node.high.y {
		return false
	}

	if node.left != nil {
		return node.left.overlaps(ctx, triangle, identity) || node.right.overlaps(ctx, triangle, identity)
	}

	for i := range node.triangles {
		other := &node.triangles[i]
		if other == identity {
			continue
		}

		if triangle.high.x <= other.low.x || triangle.low.x >= other.high.x || triangle.high.y <= other.low.y ||
			triangle.low.y >= other.high.y {
			continue
		}

		if uvIntersects(triangle, *other) {
			return true
		}
	}

	return false
}
func chartOverlaps(ctx context.Context, mesh rawMesh, faces []uint32) bool {
	checkRepairContext(ctx)

	if len(faces) < 2 {
		return false
	}

	triangles := make([]uvTriangle, len(faces))
	for i, face := range faces {
		if i%256 == 0 {
			checkRepairContext(ctx)
		}

		triangle := &triangles[i]
		triangle.low = uvPoint{math.Inf(1), math.Inf(1)}
		triangle.high = uvPoint{math.Inf(-1), math.Inf(-1)}

		for corner := range 3 {
			vertex := mesh.vertices[mesh.indices[face*3+uint32(corner)]]
			part := uvPoint{float64(vertex.uv[0]), float64(vertex.uv[1])}
			triangle.points[corner] = part
			triangle.low.x = min(triangle.low.x, part.x)
			triangle.low.y = min(triangle.low.y, part.y)
			triangle.high.x = max(triangle.high.x, part.x)
			triangle.high.y = max(triangle.high.y, part.y)
		}
	}

	tree := uvTree(ctx, triangles)
	for i := range triangles {
		if tree.overlaps(ctx, triangles[i], &triangles[i]) {
			return true
		}
	}

	return false
}
func worldEdges(mesh rawMesh, face uint32, source []byte) (ab, ac [3]float64, area float64) {
	var points [3][3]float64

	for corner := range 3 {
		original := mesh.vertices[mesh.indices[face*3+uint32(corner)]].original
		if uint64(original)*32+12 > uint64(len(source)) {
			panic(errFormat)
		}

		for axis := range 3 {
			points[corner][axis] = float64(math.Float32frombits(binary.LittleEndian.Uint32(source[int(original)*32+axis*4:])))
		}
	}

	for axis := range 3 {
		ab[axis] = points[1][axis] - points[0][axis]
		ac[axis] = points[2][axis] - points[0][axis]
	}

	for axis := range 3 {
		x, y := (axis+1)%3, (axis+2)%3
		cross := ab[x]*ac[y] - ab[y]*ac[x]
		area += cross * cross
	}

	return ab, ac, math.Sqrt(area)
}
func chartCollapsed(ctx context.Context, mesh rawMesh, faces []uint32, source []byte) bool {
	for index, face := range faces {
		if index%256 == 0 {
			checkRepairContext(ctx)
		}

		var coordinates [3]uvPoint

		for corner := range 3 {
			vertex := mesh.vertices[mesh.indices[face*3+uint32(corner)]]
			coordinates[corner] = uvPoint{float64(vertex.uv[0]), float64(vertex.uv[1])}
		}

		if uvSide(coordinates[0], coordinates[1], coordinates[2]) == 0 {
			_, _, area := worldEdges(mesh, face, source)
			if area > 0 {
				return true
			}
		}
	}

	return false
}

func (guest atlasGuest) repair(
	address, pack uint32,
	atlas rawAtlas,
	sources [][]byte,
	density float32,
	parts, maxPages, resolution uint32,
) rawAtlas {
	invalid, needed := guest.invalidCharts(atlas, sources)
	if !needed {
		return atlas
	}

	next := guest.call("xatlasCreate")
	material := atlas.charts

	maps := make([][]uint32, len(atlas.meshes))
	for part, mesh := range atlas.meshes {
		if err := guest.ctx.Err(); err != nil {
			panic(err)
		}

		uvs, refs, indices, materials, nextMaterial := guest.repairMesh(mesh, sources[part], invalid, material, density)
		material = nextMaterial
		maps[part] = refs

		guest.addRepairMesh(next, uvs, indices, materials)
	}

	guest.call("xatlasComputeCharts", uint64(next), 0)
	guest.putf(pack+8, 1.0/1024)
	guest.write(pack+20, []byte{0})
	guest.call("xatlasPackCharts", uint64(next), uint64(pack))

	result := guest.atlas(next, parts, maxPages, resolution)
	for part := range result.meshes {
		remapRepairedMesh(guest.ctx, &result.meshes[part], maps[part])
	}

	guest.call("xatlasDestroy", uint64(address))
	guest.call("xatlasDestroy", uint64(next))

	return result
}

// Host-side repair must obey the same deadline as active WASM execution.
// execute catches this request-local abort and returns no partial output.
func checkRepairContext(ctx context.Context) {
	if err := ctx.Err(); err != nil {
		panic(err)
	}
}
