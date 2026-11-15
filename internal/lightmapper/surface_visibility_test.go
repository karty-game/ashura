package lightmapper

import (
	"math/rand/v2"
	"testing"
)

//nolint:gosec // G404: deterministic geometry/ray fixtures need reproducibility, not cryptographic randomness.
func TestSurfaceVisibilityMatchesReferenceTraversal(t *testing.T) {
	t.Parallel()

	random := rand.New(rand.NewPCG(19, 37))
	vector := func() Vec3 {
		return Vec3{random.Float32()*4 - 2, random.Float32()*4 - 2, random.Float32()*4 - 2}
	}

	triangles := make([]Triangle, 256)
	for index := range triangles {
		triangles[index] = Triangle{
			ID: uint64(index + 1), Positions: [3]Vec3{vector(), vector(), vector()},
			Normals: [3]Vec3{{0, 0, 1}, {0, 0, 1}, {0, 0, 1}},
		}
	}

	scene, err := PrepareScene(Scene{
		ID: "surface-visibility", Space: "world", MetresPerUnit: 1,
		Triangles: triangles, Materials: []Material{{Reflectance: Vec3{.5, .5, .5}}},
	})
	if err != nil {
		t.Fatal(err)
	}

	visibility := newSurfaceVisibility(scene)

	var candidates []uint32

	var localQueries int

	for index := range 20000 {
		direction := Normalize(vector())
		// Axis-parallel and almost-parallel directions exercise slab handling.
		switch index % 4 {
		case 0:
			direction = Vec3{1, 0, 0}
		case 1:
			direction = Vec3{1, 1e-25, -1e-21}
		}

		ray := Ray{Origin: vector(), Direction: direction, Min: 1e-6, Max: random.Float32()*4 + 1e-5}
		if index%3 == 0 {
			ray.Max = .01
		}

		if got, want := visibility.occluded(ray), scene.Intersect(ray, true).Found; got != want {
			t.Fatal("changed complete-scene visibility", index, ray, got, want)
		}

		end := Add(ray.Origin, Scale(ray.Direction, ray.Max))

		var local bool

		candidates, local = visibility.candidates(minVec(ray.Origin, end), maxVec(ray.Origin, end), candidates)
		if local {
			localQueries++

			if got, want := visibility.occludedCandidates(ray, candidates), scene.Intersect(ray, true).Found; got != want {
				t.Fatal("local candidate query omitted an occluder", index, ray, got, want)
			}
		}
	}

	if localQueries == 0 {
		t.Fatal("fixture never exercised bounded local candidate lists")
	}

	if _, local := visibility.candidates(Vec3{-5, -5, -5}, Vec3{5, 5, 5}, candidates); local {
		t.Fatal("large candidate set did not select complete BVH fallback")
	}
}

func TestSurfaceVisibilityRetainsThinWallOcclusion(t *testing.T) {
	t.Parallel()
	scene, _ := surfaceEdgeFixture(t)
	source := scene.Source()
	source.Triangles = append(source.Triangles, Triangle{
		ID: 3, Positions: [3]Vec3{{.5, -1, -.01}, {.5, 2, -.01}, {.5, .5, .01}},
		Normals: [3]Vec3{{1, 0, 0}, {1, 0, 0}, {1, 0, 0}},
	}, Triangle{
		ID: 4, Positions: [3]Vec3{{3, 0, 0}, {4, 0, 0}, {3, 1, 0}},
		Normals: [3]Vec3{{0, 0, 1}, {0, 0, 1}, {0, 0, 1}},
	})

	scene, err := PrepareScene(source)
	if err != nil {
		t.Fatal(err)
	}

	visibility := newSurfaceVisibility(scene)
	first := Receiver{Position: Vec3{.25, .5, 0}, GeometricNormal: Vec3{0, 0, 1}}

	second := Receiver{Position: Vec3{.75, .5, 0}, GeometricNormal: Vec3{0, 0, 1}}
	if visibility.visible(first, second, 0) || visibility.visible(second, first, 0) {
		t.Fatal("surface support crossed the thin wall")
	}

	ray := surfaceVisibilityRay(first, second, 0)
	end := Add(ray.Origin, Scale(ray.Direction, ray.Max))

	candidates, local := visibility.candidates(minVec(ray.Origin, end), maxVec(ray.Origin, end), nil)
	if !local || !visibility.occludedCandidates(ray, candidates) {
		t.Fatal("local owner query omitted the thin wall")
	}

	if len(candidates) >= len(scene.order) {
		t.Fatal("loose leaf sibling bypassed original triangle bounds", candidates)
	}
}
