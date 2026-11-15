package lightmapper

import (
	"math"
	"testing"
)

func TestSceneSAHVisibilityAndStackBounds(t *testing.T) {
	t.Parallel()

	source := testCoreScene()
	source.Materials = []Material{{Reflectance: Vec3{.5, .5, .5}}}

	for triangleIndex := range 2048 {
		x, y, z := float32((triangleIndex*37)%97)*.2, float32((triangleIndex*13)%89)*.2, float32(triangleIndex%7)+1
		source.Triangles = append(
			source.Triangles,
			Triangle{ID: uint64(triangleIndex + 1), Positions: [3]Vec3{{x, y, z}, {x + .15, y, z}, {x, y + .15, z}}, Material: 0},
		)
	}

	prepared, err := PrepareScene(source)
	if err != nil {
		t.Fatal(err)
	}

	seen := make([]bool, len(source.Triangles))

	var walk func(uint32, int)

	walk = func(index uint32, depth int) {
		if depth >= 64 {
			t.Fatal("traversal stack overflow", depth)
		}

		node := prepared.nodes[index]
		if node.Count == 0 {
			walk(node.Left, depth+1)
			walk(node.Right, depth+1)

			return
		}

		for _, id := range prepared.order[node.First : node.First+node.Count] {
			if seen[id] {
				t.Fatal("duplicate BVH triangle", id)
			}

			seen[id] = true
		}
	}
	walk(0, 0)

	for triangleIndex, v := range seen {
		if !v {
			t.Fatal("missing BVH triangle", triangleIndex)
		}
	}

	for triangleIndex := 0; triangleIndex < len(source.Triangles); triangleIndex += 31 {
		t0 := source.Triangles[triangleIndex]
		origin := Scale(Add(t0.Positions[0], Add(t0.Positions[1], t0.Positions[2])), 1.0/3)
		origin[2] = 0
		ray := Ray{Origin: origin, Direction: Vec3{0, 0, 1}, Min: 1e-6, Max: 100}
		expected := float32(100)
		found := false

		for _, triangle := range source.Triangles {
			d, _, hit := rayTriangle(ray, triangle)
			if hit && d < expected {
				expected = d
				found = true
			}
		}

		hit := prepared.Intersect(ray, false)
		if hit.Found != found || found && math.Abs(float64(hit.Distance-expected)) > 1e-5 || prepared.Intersect(ray, true).Found != found {
			t.Fatal("SAH changed visibility", triangleIndex, hit, expected)
		}
	}
}

func TestSceneCodecPreservesMaterialIdentityAndNormalizedFrames(t *testing.T) {
	t.Parallel()

	source := testCoreScene()
	source.Materials = []Material{{ID: "object/material", Reflectance: Vec3{.5, .4, .3}}}
	source.Triangles = []Triangle{
		{ID: 1, Positions: [3]Vec3{{0, 0, 0}, {1, 0, 0}, {0, 1, 0}}, Normals: [3]Vec3{{1, 2, 3}, {1, 2, 3}, {1, 2, 3}}, Material: 0},
	}
	source.Emitters = []Emitter{
		{
			ID:        2,
			Kind:      Rectangle,
			Position:  Vec3{0, 0, 2},
			Direction: Vec3{1, 2, 3},
			Tangent:   Vec3{1, 1, 1},
			Size:      Vec2{1, 2},
			Intensity: Vec3{1, 1, 1},
		},
	}

	prepared, err := PrepareScene(source)
	if err != nil {
		t.Fatal(err)
	}

	bytes, err := EncodeScene(prepared)
	if err != nil {
		t.Fatal(err)
	}

	decoded, err := DecodeScene(bytes)
	if err != nil {
		t.Fatal(err)
	}

	if decoded.Hash() != prepared.Hash() || decoded.Source().Materials[0].ID != "object/material" {
		t.Fatal("codec changed scene identity", prepared.Hash(), decoded.Hash())
	}

	source.Materials = append(source.Materials, source.Materials[0])
	if _, err = PrepareScene(source); err == nil {
		t.Fatal("duplicate material identity admitted")
	}
}

func TestGeometryTraversalRetainsThinNondegenerateTriangle(t *testing.T) {
	t.Parallel()

	source := testCoreScene()
	source.Materials = []Material{{Reflectance: Vec3{.5, .5, .5}}}
	source.Triangles = []Triangle{{ID: 1, Positions: [3]Vec3{{0, 0, 0}, {1, 0, 0}, {0, 1e-9, 0}}, Material: 0}}

	prepared, err := PrepareScene(source)
	if err != nil {
		t.Fatal(err)
	}

	ray := Ray{Origin: Vec3{.25, 2.5e-10, 1}, Direction: Vec3{0, 0, -1}, Min: 1e-6, Max: 10}
	if hit := prepared.Intersect(ray, false); !hit.Found || math.Abs(float64(hit.Distance-1)) > 1e-6 {
		t.Fatal("thin surface lost", hit)
	}
}
