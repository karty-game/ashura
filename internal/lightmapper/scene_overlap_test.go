package lightmapper

import (
	"errors"
	"math"
	"strings"
	"testing"
)

func testRectEmitter(id uint64, x, width float32) Emitter {
	return Emitter{
		ID:        id,
		Kind:      Rectangle,
		Position:  Vec3{x, 0, 2},
		Direction: Vec3{0, 0, -1},
		Tangent:   Vec3{1, 0, 0},
		Size:      Vec2{width, 2},
		Intensity: Vec3{1, 1, 1},
	}
}

func TestCoincidentEmittingSurfacesFailExplicitly(t *testing.T) {
	t.Parallel()

	source := testCoreScene()

	source.Emitters = []Emitter{testRectEmitter(1, 0, 2), testRectEmitter(2, 0, 2)}
	if _, err := PrepareScene(source); !errors.Is(err, ErrUnsupported) || !strings.Contains(err.Error(), "unique emission owner") {
		t.Fatal("ambiguous MIS emission admitted", err)
	}

	source.Emitters = []Emitter{
		{ID: 1, Kind: Sphere, Position: Vec3{0, 0, 2}, Radius: 1, Intensity: Vec3{1, 1, 1}},
		{ID: 2, Kind: Sphere, Position: Vec3{0, 0, 2}, Radius: 1, Intensity: Vec3{1, 1, 1}},
	}
	if _, err := PrepareScene(source); !errors.Is(err, ErrUnsupported) {
		t.Fatal("coincident spheres admitted", err)
	}
}

func TestAnalyticEmitterCannotSilentlyShareOpaqueWall(t *testing.T) {
	t.Parallel()

	source := testCoreScene()
	source.Emitters = []Emitter{testRectEmitter(2, 0, 1)}
	source.Materials = []Material{{Reflectance: Vec3{.5, .5, .5}}}

	source.Triangles = []Triangle{{ID: 1, Positions: [3]Vec3{{-2, -2, 2}, {2, -2, 2}, {0, 2, 2}}, Material: 0}}
	if _, err := PrepareScene(source); !errors.Is(err, ErrUnsupported) || !strings.Contains(err.Error(), "use that triangle's emission") {
		t.Fatal("coplanar wall/source ownership admitted", err)
	}

	source.Emitters = nil

	source.Materials[0].Emission = Vec3{1, 1, 1}
	if _, err := PrepareScene(source); err != nil {
		t.Fatal("unambiguous triangle-owned emission rejected", err)
	}
}

func TestDisjointAreaPartitionPreservesMISAndEnergy(t *testing.T) {
	t.Parallel()

	source := testCoreScene()
	source.Emitters = []Emitter{testRectEmitter(1, 0, 2)}

	whole, err := PrepareScene(source)
	if err != nil {
		t.Fatal(err)
	}

	source.Emitters = []Emitter{testRectEmitter(1, -.5, 1), testRectEmitter(2, .5, 1)}

	split, err := PrepareScene(source)
	if err != nil {
		t.Fatal("shared edge treated as positive-area overlap", err)
	}

	wholeResult, err := BakeReference(t.Context(), whole, []Receiver{testPointReceiver()}, Options{Samples: 4096, Seed: 17})
	if err != nil {
		t.Fatal(err)
	}

	splitResult, err := BakeReference(t.Context(), split, []Receiver{testPointReceiver()}, Options{Samples: 4096, Seed: 17})
	if err != nil {
		t.Fatal(err)
	}

	if math.Abs(float64(wholeResult.Estimates[0].Statistics[0][0]-splitResult.Estimates[0].Statistics[0][0])) > .006 {
		t.Fatal("area partition changed emitted energy", wholeResult.Estimates[0].Statistics[0], splitResult.Estimates[0].Statistics[0])
	}
}

func TestEmitterOwnershipPreparationUsesSpatialHierarchyFor16384SparseAreas(t *testing.T) {
	t.Parallel()

	source := testCoreScene()

	source.Emitters = make([]Emitter, 16384)
	for i := range source.Emitters {
		source.Emitters[i] = testRectEmitter(uint64(i+1), float32(i%128)*3, .25)
		source.Emitters[i].Position[1] = float32(i/128) * 3
		source.Emitters[i].Size[1] = .25
	}

	if _, err := PrepareScene(source); err != nil {
		t.Fatal(err)
	}
}
