package engine

import (
	"testing"

	"github.com/karty-game/ashura/internal/render"
)

type geometryFixture struct {
	positions, normals []Vector3
	uvs                []Vector2
	indices            []uint32
}

func (fixture geometryFixture) RuntimeMesh() ([]Vector3, []uint32, []Vector3, []Vector2, bool) {
	return fixture.positions, fixture.indices, fixture.normals, fixture.uvs, false
}

func fixtureTriangle() geometryFixture {
	return geometryFixture{positions: []Vector3{{}, {X: 1}, {Y: 1}}, indices: []uint32{0, 1, 2}}
}

func geometryFrame(t *testing.T, world *World) render.Frame {
	t.Helper()

	frame, err := makeFrame(world, 100, 100)
	if err != nil {
		t.Fatal(err)
	}

	// makeFrame borrows reusable draw storage; retain this test snapshot explicitly.
	frame.Draws = append([]render.DrawCall(nil), frame.Draws...)

	return frame
}

func TestGeometrySurvivesCameraAndObjectMovement(t *testing.T) {
	t.Parallel()

	world := NewWorld()
	view := world.Add(DefaultTransform(), perspectiveFixture{})
	entity := world.Add(DefaultTransform(), fixtureTriangle())

	first := geometryFrame(t, world)
	for step := range 100 {
		world.Transform(entity).Position.X = float32(step + 1)
		world.Transform(view).Position.Z = float32(step + 1)

		frame := geometryFrame(t, world)
		if &frame.Draws[0].Vertices[0] != &first.Draws[0].Vertices[0] || &frame.Draws[0].Indices[0] != &first.Draws[0].Indices[0] {
			t.Fatal("movement rebuilt static geometry")
		}

		if frame.Draws[0].Transform == first.Draws[0].Transform || frame.ViewProjection == first.ViewProjection {
			t.Fatal("caching stopped camera/object updates")
		}
	}
}

func TestGeometrySharedReplacementAndRemoval(t *testing.T) {
	t.Parallel()

	world := NewWorld()
	geometry := fixtureTriangle()
	first := world.Add(DefaultTransform(), geometry)
	second := world.Add(DefaultTransform(), geometry)

	frame := geometryFrame(t, world)
	if &frame.Draws[0].Vertices[0] != &frame.Draws[1].Vertices[0] || len(world.geometry) != 1 {
		t.Fatal("shared source geometry was converted more than once")
	}

	if !world.Remove(first) {
		t.Fatal("remove failed")
	}

	remaining := geometryFrame(t, world)
	if len(remaining.Draws) != 1 || &remaining.Draws[0].Vertices[0] != &frame.Draws[0].Vertices[0] {
		t.Fatal("removing a shared user invalidated the surviving geometry")
	}

	geometry.normals = []Vector3{{Y: 1}, {Y: 1}, {Y: 1}}
	if err := world.Set(second, geometry); err != nil {
		t.Fatal(err)
	}

	replaced := geometryFrame(t, world)
	if len(world.geometry) != 1 || &replaced.Draws[0].Vertices[0] == &remaining.Draws[0].Vertices[0] ||
		replaced.Draws[0].Vertices[0].Normal != [3]float32{0, 1, 0} {
		t.Fatal("normal slice replacement reused stale geometry or retained unused records")
	}

	if !world.Remove(second) {
		t.Fatal("remove failed")
	}

	if len(geometryFrame(t, world).Draws) != 0 || len(world.geometry) != 0 {
		t.Fatal("last removal retained geometry")
	}
}

func TestGeometrySliceLengthAndIndexReplacement(t *testing.T) {
	t.Parallel()

	world := NewWorld()
	geometry := fixtureTriangle()
	geometry.positions = append(geometry.positions, Vector3{Z: 1})
	entity := world.Add(geometry)
	first := geometryFrame(t, world)
	stored, _ := Get[geometryFixture](world, entity)
	stored.positions = stored.positions[:3]
	stored.indices = []uint32{2, 1, 0}

	frame := geometryFrame(t, world)
	if len(frame.Draws[0].Vertices) != 3 || frame.Draws[0].Indices[0] != 2 ||
		&frame.Draws[0].Vertices[0] == &first.Draws[0].Vertices[0] || len(world.geometry) != 1 {
		t.Fatal("slice header replacement did not select fresh geometry")
	}
}
