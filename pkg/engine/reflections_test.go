package engine

import (
	"testing"

	"github.com/karty-game/karty-webgpu/internal/render"
	"github.com/karty-game/karty-webgpu/internal/texturedata"
)

type reflectedMaterialFixture struct{ strength float32 }

func (surface reflectedMaterialFixture) RuntimeReflectionStrength() float32 { return surface.strength }

type reflectionProbeFixture struct {
	image    *texturedata.Texture
	half     Vector3
	strength float32
}

func (probe reflectionProbeFixture) RuntimeReflectionProbe() (*texturedata.Texture, Vector3, float32) {
	return probe.image, probe.half, probe.strength
}

func reflectionImage(t *testing.T) *texturedata.Texture {
	t.Helper()

	image, err := texturedata.FromOwned(texturedata.Data{
		Width: 6, Height: 1, ColorSpace: texturedata.SRGB, Levels: [][]byte{make([]byte, 24)},
	})
	if err != nil {
		t.Fatal(err)
	}

	return image
}

//nolint:paralleltest // AllocsPerRun must be isolated on Go 1.27.
func TestReflectedInstancesDoNotAllocate(t *testing.T) {
	world := NewWorld()
	view := world.Add(DefaultTransform(), perspectiveFixture{})
	image := reflectionImage(t)
	probe := world.Add(DefaultTransform(), reflectionProbeFixture{
		image: image, half: Vector3{X: 100, Y: 100, Z: 100}, strength: 1,
	})
	geometry, source := fixtureTriangle(), new(int)

	var moving Entity

	const instances = 1000
	for range instances {
		moving = world.Add(
			DefaultTransform(),
			instanceFixture{geometryFixture: geometry, source: source},
			reflectedMaterialFixture{strength: 1},
		)
	}

	for range 3 {
		if _, err := makeFrame(world, 100, 100); err != nil {
			t.Fatal(err)
		}
	}

	for _, move := range []bool{false, true} {
		allocations := testing.AllocsPerRun(100, func() {
			if move {
				world.Transform(moving).Position.Y += .01
				world.Transform(view).Position.Z += .01
			}

			frame, err := makeFrame(world, 100, 100)
			if err != nil || len(frame.Draws) != 1 || len(frame.Draws[0].Instances) != instances || frame.Draws[0].ProbeID != probe.index {
				t.Fatal("reflected instance admission/grouping changed", err)
			}
		})
		if allocations != 0 {
			t.Fatalf("moving=%v: reflected instances allocated %v times", move, allocations)
		}
	}
}

func TestReflectionGroupsPreserveProbeVolumesAndTies(t *testing.T) {
	t.Parallel()

	world := NewWorld()
	image := reflectionImage(t)
	large := world.Add(DefaultTransform(), reflectionProbeFixture{
		image: image, half: Vector3{X: 100, Y: 100, Z: 100}, strength: 1,
	})
	left := world.Add(Transform{Position: Vector3{X: -10}}, reflectionProbeFixture{
		image: image, half: Vector3{X: 4, Y: 4, Z: 4}, strength: 1,
	})
	tie := world.Add(Transform{Position: Vector3{X: -10}}, reflectionProbeFixture{
		image: image, half: Vector3{X: 4, Y: 4, Z: 4}, strength: 1,
	})
	right := world.Add(Transform{Position: Vector3{X: 10}}, reflectionProbeFixture{
		image: image, half: Vector3{X: 4, Y: 4, Z: 4}, strength: 1,
	})
	geometry, source := fixtureTriangle(), new(int)

	var moving Entity

	for _, x := range []float32{-10, -9, 10, 11, 40} {
		transform := DefaultTransform()
		transform.Position.X = x

		entity := world.Add(transform, instanceFixture{geometryFixture: geometry, source: source}, reflectedMaterialFixture{strength: 1})
		if x == -9 {
			moving = entity
		}
	}

	frame := geometryFrame(t, world)
	assertReflectionGroups(t, frame, map[uint32]int{left.index: 2, right.index: 2, large.index: 1})

	world.Transform(moving).Position.X = 10
	assertReflectionGroups(t, geometryFrame(t, world), map[uint32]int{left.index: 1, right.index: 3, large.index: 1})
	first, _ := Get[reflectionProbeFixture](world, left)
	first.strength = 0

	assertReflectionGroups(t, geometryFrame(t, world), map[uint32]int{tie.index: 1, right.index: 3, large.index: 1})
	world.Transform(right).Position.X = 60
	assertReflectionGroups(t, geometryFrame(t, world), map[uint32]int{tie.index: 1, large.index: 4})
}

func assertReflectionGroups(t *testing.T, frame render.Frame, counts map[uint32]int) {
	t.Helper()

	if len(frame.Draws) != len(counts) {
		t.Fatalf("reflection draws=%d, want %d", len(frame.Draws), len(counts))
	}

	for _, draw := range frame.Draws {
		if count, found := counts[draw.ProbeID]; !found || len(draw.Instances) != count {
			t.Fatalf("probe %d has %d instances, want %v", draw.ProbeID, len(draw.Instances), counts)
		}

		delete(counts, draw.ProbeID)
	}
}
