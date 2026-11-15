//go:build !js

//nolint:mnd,varnamelen // Numeric offsets encode the GPU ABI; coordinate names follow barycentric/BVH equations.
package lightmapbuild

import (
	"encoding/binary"
	"math"
	"sort"

	"github.com/karty-game/karty-webgpu/internal/modeldata"
)

type triangle struct {
	points  [3]vec
	center  vec
	ordinal int
	part    int
}
type bounds struct{ minimum, maximum vec }

func emptyBounds() bounds {
	return bounds{
		vec{float32(math.Inf(1)), float32(math.Inf(1)), float32(math.Inf(1))},
		vec{float32(math.Inf(-1)), float32(math.Inf(-1)), float32(math.Inf(-1))},
	}
}
func (b *bounds) include(v vec) {
	for i := range 3 {
		b.minimum[i] = min(b.minimum[i], v[i])
		b.maximum[i] = max(b.maximum[i], v[i])
	}
}
func collectTriangles(parts []modeldata.Part) []triangle {
	var out []triangle

	for partIndex, part := range parts {
		for offset := 0; offset < len(part.Indices); offset += 12 {
			t := triangle{ordinal: len(out), part: partIndex}

			for j := range 3 {
				index := binary.LittleEndian.Uint32(part.Indices[offset+j*4:])
				t.points[j] = vertex(part.Vertices, index, 0)
			}

			if lengthSquared(cross(sub(t.points[1], t.points[0]), sub(t.points[2], t.points[0]))) < 1e-18 {
				continue
			}

			for axis := range 3 {
				t.center[axis] = (t.points[0][axis] + t.points[1][axis] + t.points[2][axis]) / 3
			}

			out = append(out, t)
		}
	}

	return out
}

type bvhNode struct {
	bounds

	first, count uint32
}

//nolint:gosec // The model admission bounds triangle and node counts well below uint32 before packing.
func buildBVH(triangles []triangle) ([]byte, []byte) {
	nodes := []bvhNode{{}}

	var build func(int, int, int)

	build = func(index, start, end int) {
		b, c := emptyBounds(), emptyBounds()

		for _, t := range triangles[start:end] {
			for _, p := range t.points {
				b.include(p)
			}

			c.include(t.center)
		}

		nodes[index].bounds = b
		if end-start <= 8 {
			nodes[index].first = uint32(start)
			nodes[index].count = uint32(end - start)

			return
		}

		axis := 0
		for i := 1; i < 3; i++ {
			if c.maximum[i]-c.minimum[i] > c.maximum[axis]-c.minimum[axis] {
				axis = i
			}
		}

		sort.Slice(triangles[start:end], func(i, j int) bool {
			a, b := triangles[start+i], triangles[start+j]
			if a.center[axis] == b.center[axis] {
				return a.ordinal < b.ordinal
			}

			return a.center[axis] < b.center[axis]
		})

		first := len(nodes)
		nodes[index].first = uint32(first)
		nodes = append(nodes, bvhNode{}, bvhNode{})
		middle := (start + end) / 2
		build(first, start, middle)
		build(first+1, middle, end)
	}
	build(0, 0, len(triangles))

	return packBVH(nodes, triangles)
}

func packBVH(nodes []bvhNode, triangles []triangle) ([]byte, []byte) {
	nodeBytes := make([]byte, len(nodes)*32)
	for i, n := range nodes {
		data := nodeBytes[i*32:]
		for j := range 3 {
			putFloat(data[j*4:], math.Nextafter32(n.minimum[j], float32(math.Inf(-1))))
			putFloat(data[16+j*4:], math.Nextafter32(n.maximum[j], float32(math.Inf(1))))
		}

		binary.LittleEndian.PutUint32(data[12:], n.first)
		binary.LittleEndian.PutUint32(data[28:], n.count)
	}

	triangleBytes := make([]byte, len(triangles)*48)
	for i, t := range triangles {
		for j, p := range t.points {
			for k, v := range p {
				putFloat(triangleBytes[i*48+j*16+k*4:], v)
			}
		}
	}

	return nodeBytes, triangleBytes
}
