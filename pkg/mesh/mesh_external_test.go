package mesh_test

import (
	"math"
	"testing"

	engine "github.com/karty-game/karty-webgpu/pkg/engine"
	"github.com/karty-game/karty-webgpu/pkg/mesh"
)

func TestBillboardOrientationOptionsAndInstanceSharing(t *testing.T) {
	t.Parallel()

	for _, mode := range []mesh.BillboardMode{mesh.BillboardSpherical, mesh.BillboardAxis, mesh.BillboardFixed} {
		value := mesh.NewBillboard("billboard", mesh.BillboardOptions{Mode: mode, Axis: engine.Vector3{X: 1}})

		selected, axis := value.RuntimeBillboard()
		if selected != uint8(mode) || axis != (engine.Vector3{X: 1}) || !value.Billboard {
			t.Fatal("billboard mode/axis lost")
		}

		instance := value.CreateInstance("copy")
		instanceMode, instanceAxis := instance.RuntimeBillboard()

		positions, _, _, _, _ := instance.RuntimeMesh()
		if instanceMode != selected || instanceAxis != axis || &positions[0] != &value.Data.Positions[0] {
			t.Fatal("instance did not share billboard geometry/settings")
		}
	}

	value := mesh.NewBillboard("default", mesh.BillboardOptions{})

	mode, axis := value.RuntimeBillboard()
	if mode != uint8(mesh.BillboardSpherical) || axis != engine.Vector3Up() {
		t.Fatal("billboard defaults are not spherical/+Y")
	}
}

func TestMeshConstructorsAndCustomValidation(t *testing.T) {
	t.Parallel()

	constructors := []struct {
		name      string
		billboard bool
		make      func() mesh.Mesh
	}{
		{name: "box", make: func() mesh.Mesh { return mesh.NewBox("box", mesh.BoxOptions{}) }},
		{name: "sphere", make: func() mesh.Mesh { return mesh.NewSphere("sphere", mesh.SphereOptions{}) }},
		{name: "plane", make: func() mesh.Mesh { return mesh.NewPlane("plane", mesh.PlaneOptions{}) }},
		{
			name:      "billboard",
			billboard: true,
			make:      func() mesh.Mesh { return mesh.NewBillboard("billboard", mesh.BillboardOptions{}) },
		},
		{name: "cylinder", make: func() mesh.Mesh { return mesh.NewCylinder("cylinder", mesh.CylinderOptions{}) }},
		{name: "capsule", make: func() mesh.Mesh { return mesh.NewCapsule("capsule", mesh.CapsuleOptions{}) }},
	}
	for _, constructor := range constructors {
		t.Run(constructor.name, func(t *testing.T) {
			t.Parallel()

			got := constructor.make()

			if got.Name != constructor.name || len(got.Data.Positions) == 0 || len(got.Data.Indices) == 0 {
				t.Fatalf("constructor returned incomplete mesh: %#v", got)
			}

			if got.Billboard != constructor.billboard {
				t.Fatalf("Billboard = %v, want %v", got.Billboard, constructor.billboard)
			}

			if len(got.Data.Indices)%3 != 0 {
				t.Fatalf("index count = %d, want triangle list", len(got.Data.Indices))
			}

			for _, index := range got.Data.Indices {
				if int(index) >= len(got.Data.Positions) {
					t.Fatalf("index %d is outside %d positions", index, len(got.Data.Positions))
				}
			}
		})
	}

	positions := []engine.Vector3{{X: 0}, {X: 1}, {Y: 1}}
	indices := []uint32{0, 1, 2}

	got, err := mesh.NewCustom("custom", mesh.Data{Positions: positions, Indices: indices})
	if err != nil {
		t.Fatal(err)
	}

	positions[0].X = 99
	indices[0] = 2

	if got.Data.Positions[0].X != 0 || got.Data.Indices[0] != 0 {
		t.Fatal("custom mesh retained caller-owned slices")
	}

	for _, invalid := range []mesh.Data{
		{Positions: []engine.Vector3{{X: float32(math.NaN())}, {X: 1}, {Y: 1}}, Indices: []uint32{0, 1, 2}},
		{Positions: []engine.Vector3{{}, {X: 1}, {Y: 1}}, Indices: []uint32{0, 1}},
		{Positions: []engine.Vector3{{}, {X: 1}, {Y: 1}}, Indices: []uint32{0, 1, 3}},
		{Positions: []engine.Vector3{{}, {X: 1}, {Y: 1}}, Indices: []uint32{0, 1, 2}, Normals: []engine.Vector3{{}}},
	} {
		if _, err := mesh.NewCustom("invalid", invalid); err == nil {
			t.Fatalf("NewCustom accepted invalid geometry: %#v", invalid)
		}
	}
}

func TestPrimitiveSegmentBounds(t *testing.T) {
	t.Parallel()

	for _, segments := range []uint32{1, 2, ^uint32(0)} {
		geometries := []mesh.Mesh{
			mesh.NewSphere("sphere", mesh.SphereOptions{Segments: segments}),
			mesh.NewCylinder("cylinder", mesh.CylinderOptions{Segments: segments}),
			mesh.NewCapsule("capsule", mesh.CapsuleOptions{Segments: segments}),
		}
		for _, geometry := range geometries {
			// Reuse the public custom-mesh admission path to check generated geometry.
			if _, err := mesh.NewCustom("check", geometry.Data); err != nil {
				t.Fatalf("%s with %d segments: %v", geometry.Name, segments, err)
			}
		}
	}
}
