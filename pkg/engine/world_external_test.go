package engine_test

import (
	"testing"

	"github.com/karty-game/ashura/pkg/camera"
	"github.com/karty-game/ashura/pkg/engine"
)

type health struct {
	Value int
}

func TestWorldComponentsAndRemoval(t *testing.T) {
	t.Parallel()

	world := engine.NewWorld()
	entity := world.Add(engine.DefaultTransform(), health{Value: 10})

	if !world.Alive(entity) {
		t.Fatal("new entity is not alive")
	}

	transform := world.Transform(entity)
	if transform == nil {
		t.Fatal("transform component was not attached")
	}

	transform.Position.Y = 3

	if transform.Position.Y != 3 {
		t.Fatalf("transform mutation was not retained: got %v", transform.Position.Y)
	}

	component, found := engine.Get[health](world, entity)
	if !found || component.Value != 10 {
		t.Fatalf("custom component = %#v, %v; want value 10", component, found)
	}

	if !world.Remove(entity) {
		t.Fatal("Remove returned false for a live entity")
	}

	if world.Alive(entity) {
		t.Fatal("removed entity is still alive")
	}

	if _, found = engine.Get[health](world, entity); found {
		t.Fatal("removed entity still exposes components")
	}

	if world.Remove(entity) {
		t.Fatal("removing an already removed entity succeeded")
	}
}

func TestWorldRejectsForeignEntity(t *testing.T) {
	t.Parallel()

	first, second := engine.NewWorld(), engine.NewWorld()
	entity := first.Add(health{Value: 1})

	if second.Alive(entity) || second.Remove(entity) {
		t.Fatal("foreign entity was accepted")
	}
}

func TestWorldSelectsActiveCamera(t *testing.T) {
	t.Parallel()

	world := engine.NewWorld()
	projection, _ := camera.FirstPerson()
	entity := world.Add(engine.DefaultTransform(), projection)

	if err := world.SetActiveCamera(entity); err != nil {
		t.Fatal(err)
	}

	active, found := world.ActiveCamera()
	if !found || active != entity {
		t.Fatalf("ActiveCamera() = %v, %v; want selected camera %v", active, found, entity)
	}

	if !world.Remove(entity) {
		t.Fatal("selected camera could not be removed")
	}

	if _, found := world.ActiveCamera(); found {
		t.Fatal("removed camera remained active")
	}
}
