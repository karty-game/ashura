//go:build !js

package lightmapbuild

import (
	"math"
	"math/rand/v2"
	"testing"
)

func TestNearFirstTransportTraversalMatchesOldAndBruteForce(t *testing.T) {
	t.Parallel()

	scene, rays := transportTraversalFixture()
	scene.ordered = true
	hits := 0

	for _, ray := range rays {
		hit, distance := scene.trace(ray[0], ray[1])

		old, oldDistance := oldTransportTrace(scene, ray[0], ray[1])
		if hit != old || distance != oldDistance {
			t.Fatalf("traversal changed nearest/tie result: new%d/%g old%d/%g", hit, distance, old, oldDistance)
		}

		brute := float32(math.Inf(1))

		for _, triangle := range scene.triangles {
			d := transportIntersection(triangle, ray[0], ray[1])
			if d > 1e-4 && d < brute {
				brute = d
			}
		}

		if distance != brute {
			t.Fatal("BVH culled the true closest intersection", distance, brute)
		}

		if hit >= 0 {
			hits++
		}
	}

	if hits < 500 {
		t.Fatal("fixture did not exercise enough real intersections", hits)
	}
}

func transportTraversalFixture() (transportScene, [][2]vec) {
	//nolint:gosec // Seeded fixture generation is deliberately reproducible, never cryptographic.
	random := rand.New(rand.NewPCG(31, 71))
	triangles := make([]triangle, 300)

	vector := func(size float32) vec {
		return vec{random.Float32()*size - size*.5, random.Float32()*size - size*.5, random.Float32()*size - size*.5}
	}
	for index := range triangles {
		origin := vector(20)
		first, second := vector(2), vector(2)
		triangles[index] = triangle{
			points:  [3]vec{origin, add(origin, first), add(origin, second)},
			center:  add(origin, scale(add(first, second), 1.0/3)),
			ordinal: index,
		}
	}
	// Coincident geometry must retain exactly the old right-first leaf winner.
	for index := range 30 {
		triangles[index].points = triangles[100].points
		triangles[index].center = triangles[100].center
	}

	nodes, _ := buildBVH(triangles)
	scene := newTransportScene(nodes, triangles, make([]byte, len(triangles)*16))

	rays := make([][2]vec, 2048)
	for index := range rays {
		origin := vector(40)

		direction := normalize(vector(2))
		if index%2 == 0 {
			direction = normalize(sub(triangles[index%len(triangles)].center, origin))
		}

		rays[index] = [2]vec{origin, direction}
	}

	return scene, rays
}

func oldTransportTrace(scene transportScene, origin, direction vec) (int, float32) {
	inverse := vec{1 / direction[0], 1 / direction[1], 1 / direction[2]}
	stack := [64]uint32{}
	top := 1
	nearest := float32(math.Inf(1))
	found := -1

	for top > 0 {
		top--

		node := scene.nodes[stack[top]]
		if !oldTransportBox(node.bounds, origin, inverse, nearest) {
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

func oldTransportBox(box bounds, origin, inverse vec, limit float32) bool {
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

func BenchmarkTransportTraversal(b *testing.B) {
	scene, rays := transportTraversalFixture()
	scene.ordered = true

	b.Run("old-right-first", func(b *testing.B) {
		index := 0

		b.ReportAllocs()

		for b.Loop() {
			ray := rays[index%len(rays)]
			oldTransportTrace(scene, ray[0], ray[1])

			index++
		}
	})
	b.Run("near-first", func(b *testing.B) {
		index := 0

		b.ReportAllocs()

		for b.Loop() {
			ray := rays[index%len(rays)]
			scene.trace(ray[0], ray[1])

			index++
		}
	})
}
