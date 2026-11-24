package engine

import (
	"testing"

	"github.com/karty-game/ashura/internal/render"
	"github.com/karty-game/ashura/internal/texturedata"
	"github.com/karty-game/ashura/pkg/texture"
)

type sourceBindingFixture struct {
	runtimeMaterialFixture

	albedo, normal, occlusion *texture.Texture
}

func (value sourceBindingFixture) RuntimeSource() ([3]Color, bool, float32) {
	return [3]Color{ColorRed(), ColorGreen(), ColorBlue()}, true, .7
}
func (value sourceBindingFixture) RuntimeTextures() (*texture.Texture, *texture.Texture, Vector2, float32) {
	return value.albedo, value.normal, Vector2{X: 2, Y: 3}, .7
}
func (value sourceBindingFixture) RuntimeOcclusion() (*texture.Texture, float32) {
	return value.occlusion, .5
}
func (value sourceBindingFixture) RuntimeCullBackfaces() bool { return true }

type combinedMeshMaterialFixture struct {
	geometryFixture
	runtimeMaterialFixture

	physics int
}

type secondLightFixture struct{ runtimeLightFixture }

func TestMaterialAssignmentsAndRemovalUseDeterministicProviders(t *testing.T) {
	t.Parallel()

	world := NewWorld()
	albedo := texturedata.Solid([4]byte{255, 0, 0, 255}, texture.SRGB)
	linear := texturedata.Solid([4]byte{128, 128, 255, 255}, texture.Linear)
	source := sourceBindingFixture{
		color: ColorRed(), roughness: .2,
		albedo: albedo, normal: linear, occlusion: linear,
	}
	standard := runtimeMaterialFixture{color: Color{G: 1, A: 1}, roughness: .5}
	entity := world.Add(DefaultTransform(), fixtureTriangle(), standard, source)
	assertSourceBindings(t, geometryFrame(t, world).Draws[0], albedo, linear)

	if err := world.Set(entity, standard); err != nil {
		t.Fatal(err)
	}

	assertStandardBindings(t, geometryFrame(t, world).Draws[0])

	if _, present := Get[sourceBindingFixture](world, entity); !present {
		t.Fatal("switching material silently removed an inactive component")
	}

	if err := world.Set(entity, source); err != nil {
		t.Fatal(err)
	}

	assertSourceBindings(t, geometryFrame(t, world).Draws[0], albedo, linear)

	if !RemoveComponent[sourceBindingFixture](world, entity) {
		t.Fatal("active Source component was not removed")
	}

	assertStandardBindings(t, geometryFrame(t, world).Draws[0])

	if RemoveComponent[sourceBindingFixture](world, entity) {
		t.Fatal("missing component removed twice")
	}
}

func assertSourceBindings(t *testing.T, draw render.DrawCall, albedo, linear *texture.Texture) {
	t.Helper()

	if !draw.Source || !draw.CullBackfaces || draw.Color != [4]float32{1, 0, 0, 1} ||
		draw.Albedo != albedo || draw.Normal != linear || draw.AO != linear || draw.UVScale != [2]float32{2, 3} {
		t.Fatalf("Source material provider/auxiliary mismatch: %+v", draw)
	}
}

func assertStandardBindings(t *testing.T, draw render.DrawCall) {
	t.Helper()

	if draw.Source || draw.CullBackfaces || draw.Color != [4]float32{0, 1, 0, 1} ||
		draw.Albedo != nil || draw.Normal != nil || draw.AO != nil || draw.UVScale != [2]float32{1, 1} {
		t.Fatalf("Standard retained stale Source provider/auxiliaries: %+v", draw)
	}
}

func TestLastAddWinsWithoutRemovingOtherCapabilities(t *testing.T) {
	t.Parallel()

	world := NewWorld()
	combined := combinedMeshMaterialFixture{
		geometryFixture: fixtureTriangle(), color: ColorRed(), physics: 42,
	}
	entity := world.Add(DefaultTransform(), combined, runtimeMaterialFixture{color: Color{G: 1, A: 1}})

	frame := geometryFrame(t, world)
	if len(frame.Draws) != 1 || frame.Draws[0].Color != [4]float32{0, 1, 0, 1} {
		t.Fatal("last Add material did not win or earlier mesh capability was lost")
	}

	stored, present := Get[combinedMeshMaterialFixture](world, entity)
	if !present || stored.physics != 42 {
		t.Fatal("material assignment destroyed unrelated component data")
	}

	if !RemoveComponent[runtimeMaterialFixture](world, entity) {
		t.Fatal("failed to remove active material")
	}

	if geometryFrame(t, world).Draws[0].Color != [4]float32{1, 0, 0, 1} {
		t.Fatal("material removal failed to restore previous retained provider")
	}
}

func TestComponentRemovalRefreshesCameraFallback(t *testing.T) {
	t.Parallel()

	world := NewWorld()
	first := world.Add(DefaultTransform(), perspectiveFixture{})

	second := world.Add(DefaultTransform(), perspectiveFixture{})
	if err := world.SetActiveCamera(first); err != nil {
		t.Fatal(err)
	}

	if !RemoveComponent[perspectiveFixture](world, first) {
		t.Fatal("projection removal failed")
	}

	if camera, present := activeCameraEntity(world); !present || camera != second {
		t.Fatal("projection removal retained invalid active camera")
	}

	if err := world.Set(first, perspectiveFixture{}); err != nil {
		t.Fatal(err)
	}

	if err := world.SetActiveCamera(first); err != nil {
		t.Fatal(err)
	}

	if !RemoveComponent[Transform](world, first) || world.Transform(first) != nil {
		t.Fatal("transform removal retained stale typed provider")
	}

	if camera, present := activeCameraEntity(world); !present || camera != second {
		t.Fatal("transform removal retained invalid active camera")
	}

	foreign := NewWorld()
	if RemoveComponent[Transform](foreign, second) || RemoveComponent[Transform](nil, second) {
		t.Fatal("component removal accepted foreign/nil World")
	}
}

func TestEachAndQueriesDoNotSkipAfterGroupRemoval(t *testing.T) {
	t.Parallel()

	for _, query := range []bool{false, true} {
		world := NewWorld()
		first := world.Add(DefaultTransform())
		second := world.Add(DefaultTransform())
		third := world.Add(DefaultTransform())
		fourth := world.Add(DefaultTransform())
		visited := make([]Entity, 0, 4)

		visit := func(entity Entity) {
			visited = append(visited, entity)
			if entity == second {
				world.Remove(first)
				world.Remove(second)
			}
		}
		if query {
			for entity := range Query[Transform](world) {
				visit(entity)
			}
		} else {
			world.Each(visit)
		}

		if len(visited) != 4 || visited[2] != third || visited[3] != fourth {
			t.Fatalf("query=%v: group removal skipped surviving later entities: %v", query, visited)
		}
	}
}

func TestLightProviderOrderIsStableAcrossBindingRefresh(t *testing.T) {
	t.Parallel()

	world := NewWorld()

	entity := world.Add(runtimeLightFixture{intensity: 1}, secondLightFixture{runtimeLightFixture{intensity: 2}})
	for range 10 {
		if err := world.Set(entity, DefaultTransform()); err != nil {
			t.Fatal(err)
		}

		frame := geometryFrame(t, world)
		if len(frame.Lights) != 2 || frame.Lights[0].Intensity != 1 || frame.Lights[1].Intensity != 2 {
			t.Fatal("binding refresh changed light summation order")
		}
	}
}
