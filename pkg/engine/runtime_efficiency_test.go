package engine

import (
	"errors"
	"math"
	"testing"

	"github.com/karty-game/ashura/internal/render"
)

type instanceFixture struct {
	geometryFixture

	source *int
}

func (fixture instanceFixture) RuntimeMeshIdentity() any { return fixture.source }

type billboardFixture struct {
	geometryFixture

	mode uint8
	axis Vector3
}

func (fixture billboardFixture) RuntimeMesh() ([]Vector3, []uint32, []Vector3, []Vector2, bool) {
	return fixture.positions, fixture.indices, fixture.normals, fixture.uvs, true
}
func (fixture billboardFixture) RuntimeBillboard() (uint8, Vector3) {
	return fixture.mode, fixture.axis
}

type runtimeLightFixture struct{ intensity float32 }

func (light *runtimeLightFixture) RuntimeLight() (uint32, [3]float32, [3]float32, [3]float32, float32) {
	return render.DirectionalLightKind, [3]float32{0, -1, 0}, [3]float32{1, 1, 1}, [3]float32{}, light.intensity
}

//nolint:paralleltest // AllocsPerRun must be isolated on Go 1.27.
func TestSteadyFrameDoesNotAllocate(t *testing.T) {
	for _, instanced := range []bool{false, true} {
		world := NewWorld()
		camera := world.Add(DefaultTransform(), perspectiveFixture{})
		geometry := fixtureTriangle()
		source := new(int)

		var moving Entity

		for index := range 1000 {
			transform := DefaultTransform()

			transform.Position.X = float32(index)
			if instanced {
				moving = world.Add(transform, instanceFixture{geometryFixture: geometry, source: source})
			} else {
				moving = world.Add(transform, geometry)
			}
		}

		for range 3 {
			if _, err := makeFrame(world, 100, 100); err != nil {
				t.Fatal(err)
			}
		}

		for _, move := range []bool{false, true} {
			allocations := testing.AllocsPerRun(100, func() {
				if move {
					world.Transform(moving).Position.Y++
					world.Transform(camera).Position.Z++
				}

				if _, err := makeFrame(world, 100, 100); err != nil {
					t.Fatal(err)
				}
			})
			if allocations != 0 {
				t.Fatalf("instanced=%v moving=%v: %v allocations", instanced, move, allocations)
			}
		}
	}
}

func TestTypedQueriesAndStaleEntityHandles(t *testing.T) {
	t.Parallel()

	type health struct{ value int }

	world := NewWorld()
	first := world.Add(health{value: 1}, DefaultTransform())
	second := world.Add(health{value: 2}, DefaultTransform())
	missing := world.Add(DefaultTransform())
	count := 0

	for entity, pair := range Query2[health, Transform](world) {
		pair.First.value++
		count++

		if entity == first {
			world.Remove(entity)
		}
	}

	if count != 2 || world.Alive(first) || !world.Alive(second) || !world.Alive(missing) {
		t.Fatal("typed query/removal changed live entity selection")
	}

	value, _ := Get[health](world, second)
	if value.value != 3 {
		t.Fatal("typed query did not return mutable stored component")
	}

	world.next = math.MaxUint32

	defer func() {
		recovered, ok := recover().(error)
		if !ok || !errors.Is(recovered, errEntityLimit) {
			t.Fatal("entity counter wrap did not reject handle aliasing")
		}
	}()

	world.Add(health{})
}

func TestLightsRemainBoundedAndMutationsVisible(t *testing.T) {
	t.Parallel()

	world := NewWorld()

	var last Entity
	for range render.MaxLights {
		last = world.Add(runtimeLightFixture{intensity: 1})
	}

	first := geometryFrame(t, world)
	if len(first.Lights) != render.MaxLights {
		t.Fatal("eight-light scene rejected")
	}

	light, _ := Get[runtimeLightFixture](world, last)
	light.intensity = 3

	changed := geometryFrame(t, world)
	if changed.Lights[len(changed.Lights)-1].Intensity != 3 {
		t.Fatal("direct light field write was not detected")
	}

	world.Add(runtimeLightFixture{intensity: 1})

	if _, err := makeFrame(world, 100, 100); !errors.Is(err, errTooManyLights) {
		t.Fatalf("ninth light error=%v", err)
	}

	if len(world.frame.frame.Lights) > render.MaxLights+1 {
		t.Fatal("too-many-light admission allocated an unbounded light list")
	}
}

func TestBillboardModesAndCameraRoll(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		mode uint8
		axis Vector3
	}{
		{
			name: "spherical",
		},
		{name: "axis-y", mode: 1, axis: Vector3Up()},
		{name: "axis-x", mode: 1, axis: Vector3{X: 1}},
		{name: "axis-diagonal", mode: 1, axis: Vector3{X: 1, Y: 2, Z: 3}},
		{name: "fixed", mode: 2},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			world := NewWorld()
			camera := world.Add(Transform{Position: Vector3{X: 3, Y: 2, Z: 5}}, perspectiveFixture{})
			quad := world.Add(DefaultTransform(), billboardFixture{geometryFixture: fixtureTriangle(), mode: test.mode, axis: test.axis})
			frame := geometryFrame(t, world)

			matrix := frame.Draws[0].Transform
			assertBillboardFacing(t, matrix, test.mode, test.axis, world.Transform(camera).Position)

			world.Transform(camera).Position = Vector3{Z: 5}
			world.Transform(camera).Rotation.Z = math.Pi / 2

			rolled := geometryFrame(t, world).Draws[0].Transform
			if test.mode == 0 && math.Abs(float64(rolled[4]+1)) > 1e-5 {
				t.Fatalf("spherical billboard did not follow camera roll: %v", rolled)
			}

			if test.mode == 1 && (Vector3{X: rolled[4], Y: rolled[5], Z: rolled[6]}) != unitVector(test.axis, Vector3Up()) {
				t.Fatal("camera roll changed constrained axis")
			}

			world.Transform(quad).Position = world.Transform(camera).Position

			degenerate := geometryFrame(t, world).Draws[0].Transform
			for _, value := range degenerate {
				if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
					t.Fatal("degenerate billboard produced nonfinite matrix")
				}
			}
		})
	}
}

func assertBillboardFacing(t *testing.T, matrix [16]float32, mode uint8, axis, position Vector3) {
	t.Helper()

	switch mode {
	case 2:
		if matrix != transformMatrix(DefaultTransform()) {
			t.Fatal("fixed quad changed authored orientation")
		}
	case 1:
		vertical := Vector3{X: matrix[4], Y: matrix[5], Z: matrix[6]}
		if vertical != unitVector(axis, Vector3Up()) {
			t.Fatalf("axis changed: %v", vertical)
		}
	default:
		normal := Vector3{X: matrix[8], Y: matrix[9], Z: matrix[10]}

		expected := unitVector(position, Vector3{Z: 1})
		if vectorLengthSquared(subtractVector(normal, expected)) > 1e-10 {
			t.Fatalf("billboard does not face camera: %v want %v", normal, expected)
		}
	}
}

type frameScenario struct {
	name                         string
	instances, moving, materials bool
}

func BenchmarkMakeFrame(b *testing.B) {
	for _, scenario := range []frameScenario{
		{name: "ordinary-static"}, {name: "ordinary-moving", moving: true},
		{name: "instanced-static", instances: true}, {name: "instanced-moving", instances: true, moving: true},
		{name: "many-materials-static", instances: true, materials: true}, {name: "many-materials-moving", instances: true, materials: true, moving: true},
	} {
		b.Run(scenario.name, func(b *testing.B) { benchmarkFrameScenario(b, scenario) })
	}
}

func benchmarkFrameScenario(b *testing.B, scenario frameScenario) {
	b.Helper()

	world := NewWorld()
	camera := world.Add(DefaultTransform(), perspectiveFixture{})
	geometry := fixtureTriangle()
	source := new(int)

	var moving Entity

	for index := range 5000 {
		transform := DefaultTransform()

		transform.Position.X = float32(index)

		switch {
		case scenario.materials:
			moving = world.Add(
				transform,
				instanceFixture{geometryFixture: geometry, source: source},
				runtimeMaterialFixture{color: Color{R: float32(index) / 5000, A: 1}, roughness: .5},
			)
		case scenario.instances:
			moving = world.Add(transform, instanceFixture{geometryFixture: geometry, source: source})
		default:
			moving = world.Add(transform, geometry)
		}
	}

	for range 3 {
		if _, err := makeFrame(world, 100, 100); err != nil {
			b.Fatal(err)
		}
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if scenario.moving {
			world.Transform(moving).Position.Y++

			world.Transform(camera).Position.Z++
			if scenario.materials {
				surface, _ := Get[runtimeMaterialFixture](world, moving)
				surface.roughness += .001
			}
		}

		if _, err := makeFrame(world, 100, 100); err != nil {
			b.Fatal(err)
		}
	}
}

func TestCameraBindingsFollowStructuralChanges(t *testing.T) {
	t.Parallel()

	world := NewWorld()
	earlier := world.Add(DefaultTransform())

	later := world.Add(DefaultTransform(), perspectiveFixture{})
	if selected, ok := activeCameraEntity(world); !ok || selected != later {
		t.Fatal("missing projection was selected")
	}

	if err := world.Set(earlier, perspectiveFixture{}); err != nil {
		t.Fatal(err)
	}

	if selected, ok := activeCameraEntity(world); !ok || selected != earlier {
		t.Fatal("camera structural replacement lost creation ordering")
	}

	replacement := DefaultTransform()

	replacement.Position.Z = 4
	if err := world.Set(earlier, replacement); err != nil {
		t.Fatal(err)
	}

	frame := geometryFrame(t, world)
	if frame.CameraPosition[2] != 4 {
		t.Fatal("camera binding retained replaced component pointer")
	}

	world.Remove(earlier)

	if selected, ok := activeCameraEntity(world); !ok || selected != later {
		t.Fatal("camera removal did not select surviving camera")
	}
}

type runtimeMaterialFixture struct {
	color     Color
	roughness float32
}

func (surface *runtimeMaterialFixture) RuntimeMaterial() (Color, float32) {
	return surface.color, surface.roughness
}

//nolint:paralleltest // AllocsPerRun must be isolated on Go1.27.
func TestManyMaterialGroupsRemainAllocationFree(t *testing.T) {
	world := NewWorld()
	geometry := fixtureTriangle()
	source := new(int)

	var last Entity

	const objects = 5000
	for index := range objects {
		last = world.Add(
			DefaultTransform(),
			instanceFixture{geometryFixture: geometry, source: source},
			runtimeMaterialFixture{color: Color{R: float32(index) / objects, A: 1}, roughness: .5},
		)
	}

	for range 5 {
		frame, err := makeFrame(world, 100, 100)
		if err != nil || len(frame.Draws) != objects {
			t.Fatalf("material groups=%d err=%v", len(frame.Draws), err)
		}
	}

	material, _ := Get[runtimeMaterialFixture](world, last)

	allocations := testing.AllocsPerRun(100, func() {
		material.roughness += .001

		frame, err := makeFrame(world, 100, 100)
		if err != nil || len(frame.Draws) != objects {
			t.Fatal("material mutation changed grouping cardinality")
		}
	})
	if allocations != 0 {
		t.Fatalf("warm5000material group mutations allocated %v times", allocations)
	}
}
