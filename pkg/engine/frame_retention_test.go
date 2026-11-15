package engine

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/karty-game/karty-webgpu/internal/render"
)

type unrelatedDrawFixture struct{ value int }

func TestDetachedMeshRetiresCachedDrawReferences(t *testing.T) {
	t.Parallel()

	world := NewWorld()
	entity := world.Add(DefaultTransform(), fixtureTriangle())
	geometryFrame(t, world)

	batch := world.frame.batches[entity.index]
	if batch == nil || len(batch.draw.Vertices) == 0 {
		t.Fatal("fixture did not retain converted geometry")
	}

	if !RemoveComponent[geometryFixture](world, entity) {
		t.Fatal("mesh removal failed")
	}

	if len(batch.draw.Vertices) != 0 || len(batch.draw.Indices) != 0 || batch.draw.Prepared != nil ||
		world.frame.batches[entity.index] != nil {
		t.Fatal("mesh detachment retained the entity's draw asset references")
	}

	if frame := geometryFrame(t, world); len(frame.Draws) != 0 || len(world.geometry) != 0 {
		t.Fatal("empty frame retained detached geometry")
	}

	if !world.Alive(entity) || world.Transform(entity) == nil {
		t.Fatal("mesh detachment removed unrelated entity state")
	}
}

func TestReplacingMeshWithSharedInstanceRetiresOldDraw(t *testing.T) {
	t.Parallel()

	world := NewWorld()
	source := new(int)
	geometry := fixtureTriangle()
	world.Add(DefaultTransform(), instanceFixture{geometryFixture: geometry, source: source})
	later := world.Add(DefaultTransform(), fixtureTriangle())
	geometryFrame(t, world)

	batch := world.frame.batches[later.index]
	if err := world.Set(later, instanceFixture{geometryFixture: geometry, source: source}); err != nil {
		t.Fatal(err)
	}

	if !RemoveComponent[geometryFixture](world, later) {
		t.Fatal("old mesh provider removal failed")
	}

	frame := geometryFrame(t, world)
	if len(frame.Draws) != 1 || len(frame.Draws[0].Instances) != 2 || len(world.geometry) != 1 {
		t.Fatal("replacement did not share the existing draw")
	}

	if len(batch.draw.Vertices) != 0 || len(batch.draw.Indices) != 0 || batch.draw.InstanceKey != nil {
		t.Fatal("follower instance retained its obsolete leader draw")
	}

	if !batch.matrixReady {
		t.Fatal("structural invalidation discarded reusable matrix storage")
	}
}

func TestUnrelatedComponentMutationKeepsInstanceUploadClean(t *testing.T) {
	t.Parallel()

	world := NewWorld()
	entity := world.Add(DefaultTransform(), fixtureTriangle())
	first := geometryFrame(t, world).Draws[0]
	state := render.InstanceState{}
	state.Update(first)

	if len(state.Dirty) != 1 {
		t.Fatal("initial draw did not seed instance upload state")
	}

	for _, value := range []int{1, 2} {
		if err := world.Set(entity, unrelatedDrawFixture{value: value}); err != nil {
			t.Fatal(err)
		}

		draw := geometryFrame(t, world).Draws[0]
		state.Update(draw)

		if draw.InstanceVersion != first.InstanceVersion || len(state.Dirty) != 0 {
			t.Fatal("unrelated component addition/replacement uploaded an unchanged matrix")
		}
	}

	if !RemoveComponent[unrelatedDrawFixture](world, entity) {
		t.Fatal("unrelated component removal failed")
	}

	unchanged := geometryFrame(t, world).Draws[0]
	state.Update(unchanged)

	if unchanged.InstanceVersion != first.InstanceVersion || len(state.Dirty) != 0 {
		t.Fatal("unrelated component removal uploaded an unchanged matrix")
	}

	transform := DefaultTransform()

	transform.Position.Y = 3.5
	if err := world.Set(entity, transform); err != nil {
		t.Fatal(err)
	}

	changed := geometryFrame(t, world).Draws[0]
	state.Update(changed)

	if changed.InstanceVersion == first.InstanceVersion || len(state.Dirty) != 1 ||
		state.Dirty[0] != (render.Span{Offset: 0, End: render.InstanceStride}) {
		t.Fatal("transform replacement did not upload exactly its changed matrix")
	}

	const translationYOffset = 13 * 4
	if got := binary.LittleEndian.Uint32(state.Data[translationYOffset:]); got != math.Float32bits(transform.Position.Y) {
		t.Fatal("instance upload did not encode the replaced transform")
	}
}
