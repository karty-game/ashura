package engine

import (
	"testing"

	"github.com/karty-game/ashura/internal/render"
)

type namedLightFixture struct{ data LightData }

func (fixture *namedLightFixture) LightData() LightData { return fixture.data }

type namedProjectionFixture struct{ data ProjectionData }

func (fixture *namedProjectionFixture) ProjectionData() ProjectionData { return fixture.data }

type dualLightFixture struct{ namedLightFixture }

func (*dualLightFixture) RuntimeLight() (uint32, [3]float32, [3]float32, [3]float32, float32) {
	panic("legacy provider called despite named descriptor")
}

type dualProjectionFixture struct{ namedProjectionFixture }

func (*dualProjectionFixture) RuntimeProjection() (uint8, float32, float32, float32, float32) {
	panic("legacy provider called despite named descriptor")
}

func TestNamedDescriptorsRenderAndObserveStoredFieldChanges(t *testing.T) {
	t.Parallel()

	world := NewWorld()
	projection := ProjectionData{Kind: PerspectiveProjection, FieldOfViewDegrees: 70, Near: .1, Far: 200}
	transform := DefaultTransform()
	transform.Position = Vector3{Z: 3}

	camera := world.Add(transform, dualProjectionFixture{namedProjectionFixture{data: projection}})
	if err := world.SetActiveCamera(camera); err != nil {
		t.Fatal(err)
	}

	data := LightData{
		Kind: PointLight, Position: Vector3{X: 1, Y: 2, Z: 3}, Color: ColorOrange(), Intensity: 2, Range: 25,
		Shadow: PointShadow{Enabled: true, Resolution: 512, Bias: .01, DebugFaces: true},
	}
	entity := world.Add(dualLightFixture{namedLightFixture{data: data}})

	frame := geometryFrame(t, world)
	if len(frame.Lights) != 1 {
		t.Fatalf("both descriptors were consumed for one component: %d lights", len(frame.Lights))
	}

	got := frame.Lights[0]
	if got.Kind != render.PointLightKind || got.Direction != vectorArray(data.Position) ||
		got.GroundColor != ([3]float32{25, 1, 0}) || got.Intensity != 2 ||
		!got.CastShadows || !got.DebugShadowFaces || got.ShadowResolution != 512 || got.ShadowBias != .01 {
		t.Fatalf("point descriptor lost named settings: %+v", got)
	}

	before := frame.ViewProjection
	storedLight, _ := Get[dualLightFixture](world, entity)
	storedLight.data.Intensity = 0
	storedLight.data.Position.X = 7
	storedCamera, _ := Get[dualProjectionFixture](world, camera)
	storedCamera.data.FieldOfViewDegrees = 45

	changed := geometryFrame(t, world)
	if changed.Lights[0].Intensity != 0 || changed.Lights[0].Direction[0] != 7 || changed.Lights[0].GroundColor[1] != 0 {
		t.Fatal("named light retained a snapshot instead of reading the stored component")
	}

	if changed.ViewProjection == before {
		t.Fatal("named projection mutation did not update the camera")
	}

	if !RemoveComponent[dualProjectionFixture](world, camera) {
		t.Fatal("named projection component was not removed")
	}

	if _, active := world.ActiveCamera(); active {
		t.Fatal("removed named projection left the camera selected")
	}
}

//nolint:paralleltest // AllocsPerRun must be isolated on Go 1.27.
func TestNamedDescriptorFramesDoNotAllocate(t *testing.T) {
	world := NewWorld()
	camera := world.Add(DefaultTransform(), namedProjectionFixture{ProjectionData{
		Kind: PerspectiveProjection, FieldOfViewDegrees: 60, Near: .1, Far: 1000,
	}})
	entity := world.Add(namedLightFixture{LightData{
		Kind: DirectionalLight, Direction: Vector3Down(), Color: ColorWhite(), Intensity: 1,
	}})

	for range 3 {
		geometryFrame(t, world)
	}

	light, _ := Get[namedLightFixture](world, entity)

	for _, moving := range []bool{false, true} {
		allocations := testing.AllocsPerRun(100, func() {
			if moving {
				world.Transform(camera).Position.X++
				light.data.Intensity += .01
			}

			if _, err := makeFrame(world, 100, 100); err != nil {
				t.Fatal(err)
			}
		})
		if allocations != 0 {
			t.Fatalf("named descriptors moving=%v allocated %v times", moving, allocations)
		}
	}
}
