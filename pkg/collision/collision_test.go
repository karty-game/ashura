package collision

import (
	"math"
	"testing"
	"time"

	"github.com/karty-game/ashura/pkg/engine"
)

func v(x, y, z float32) engine.Vector3 { return engine.Vector3{X: x, Y: y, Z: z} }

func quad(first, second, third, fourth engine.Vector3) *Mesh {
	bounds := Bounds{
		Min: v(
			min(first.X, second.X, third.X, fourth.X),
			min(first.Y, second.Y, third.Y, fourth.Y),
			min(first.Z, second.Z, third.Z, fourth.Z),
		),
		Max: v(
			max(first.X, second.X, third.X, fourth.X),
			max(first.Y, second.Y, third.Y, fourth.Y),
			max(first.Z, second.Z, third.Z, fourth.Z),
		),
	}

	return PreparedMeshV1([]NodeV1{{Bounds: bounds, Skip: 1, Count: 2}}, []Triangle{{first, second, third}, {first, third, fourth}})
}

func floor() *Mesh { return quad(v(-20, 0, -20), v(20, 0, -20), v(20, 0, 20), v(-20, 0, 20)) }

func wallX() *Mesh { return quad(v(0, 0, -5), v(0, 3, -5), v(0, 3, 5), v(0, 0, 5)) }

func TestRaycastNearestLayersAndRemoval(t *testing.T) {
	t.Parallel()

	var world World

	far := world.AddMesh(wallX(), 1)
	near := world.AddMesh(quad(v(-2, 0, -5), v(-2, 3, -5), v(-2, 3, 5), v(-2, 0, 5)), 2)

	start, delta := v(-4, 1, 0), v(4, 0, 0)

	hit, found := world.Raycast(start, delta, 3)
	if !found || hit.ID != near || hit.Fraction != .5 || hit.Normal != v(-1, 0, 0) || hit.Point != v(-2, 1, 0) {
		t.Fatalf("nearest ray hit = %+v, %v", hit, found)
	}

	hit, found = world.Raycast(start, delta, 1)
	if !found || hit.ID != far || hit.Fraction != 1 {
		t.Fatalf("filtered endpoint hit = %+v, %v", hit, found)
	}

	if !world.Remove(near) || world.Remove(near) {
		t.Fatal("collider removal is not idempotent")
	}

	hit, found = world.Raycast(start, delta, 3)
	if !found || hit.ID != far {
		t.Fatalf("ray after removal = %+v, %v", hit, found)
	}

	if _, found := world.Raycast(v(float32(math.NaN()), 0, 0), delta, 3); found {
		t.Fatal("nonfinite ray returned a hit")
	}
}

func TestCapsuleCastFaceEdgeAndResting(t *testing.T) {
	t.Parallel()

	var world World

	shape := Capsule{Radius: .5, HalfSegment: .5}
	wall := world.AddMesh(wallX(), 1)
	ground := world.AddMesh(floor(), 1)

	hit, found := world.CapsuleCast(v(-10, 1, 0), shape, v(20, 0, 0), 1)
	if !found || hit.ID != wall || !closeTo(hit.Fraction, .475) || hit.Normal != v(-1, 0, 0) {
		t.Fatalf("fast sweep through the wall at floor height = %+v, %v", hit, found)
	}

	hit, found = world.CapsuleCast(v(-2, 3, 0), shape, v(0, -4, 0), 1)
	if !found || hit.ID != ground || !closeTo(hit.Fraction, .5) || hit.Normal != v(0, 1, 0) {
		t.Fatalf("vertical sweep = %+v, %v", hit, found)
	}

	hit, found = world.CapsuleCast(v(-4, 1, 5.3), shape, v(8, 0, 0), 1)
	if !found || hit.ID != wall || !closeTo(hit.Fraction, .45) || !closeTo(hit.Normal.Z, .6) || !closeTo(hit.Point.Z, 5) {
		t.Fatalf("rounded edge hit = %+v, %v", hit, found)
	}

	if _, found := world.CapsuleCast(v(-4, 1, 5.6), shape, v(8, 0, 0), 1); found {
		t.Fatal("capsule should clear the wall edge")
	}

	if _, found := world.CapsuleCast(v(-.5, 1, 0), shape, v(-1, 0, 0), 1); found {
		t.Fatal("capsule could not move away from a touching wall")
	}
}

func TestCapsuleTriggerOverlapUsesRoundCaps(t *testing.T) {
	t.Parallel()

	var world World

	trigger := world.AddTrigger(Bounds{Min: v(0, 0, 0), Max: v(1, 1, 1)}, 2)
	shape := Capsule{Radius: .5, HalfSegment: .3}

	var buffer [2]ID

	if hits := world.AppendTriggers(buffer[:0], v(1.4, 1.6, 1.4), shape, 2); len(hits) != 0 {
		t.Fatalf("capsule's bounding box overlaps, but its rounded cap clears the corner: %v", hits)
	}

	if hits := world.AppendTriggers(buffer[:1], v(1.25, 1.6, 1.25), shape, 2); len(hits) != 2 || hits[1] != trigger {
		t.Fatalf("trigger appended to caller buffer = %v", hits)
	}

	if hits := world.AppendTriggers(buffer[:0], v(.5, .5, .5), shape, 1); len(hits) != 0 {
		t.Fatalf("filtered trigger = %v", hits)
	}

	if _, found := world.CapsuleCast(v(.5, 3, .5), shape, v(0, -4, 0), 2); found {
		t.Fatal("nonblocking trigger stopped a cast")
	}
}

func TestCharacterFixedStepWalkingAndSliding(t *testing.T) {
	t.Parallel()

	var world World

	world.AddMesh(floor(), 1)
	world.AddMesh(wallX(), 1)

	shape := Capsule{Radius: .4, HalfSegment: .5}
	start := v(-2, 2, -3)
	together, split := NewCharacter(shape, start), NewCharacter(shape, start)

	for range 30 {
		together.Advance(&world, 2*CharacterStep, v(4, 0, 2), 1)
		split.Advance(&world, CharacterStep, v(4, 0, 2), 1)
		split.Advance(&world, CharacterStep, v(4, 0, 2), 1)
	}

	position := together.Position()
	if position != split.Position() || !together.Grounded() || !closeTo(position.X, -.4) ||
		!closeTo(position.Y, .9) || !closeTo(position.Z, -1) {
		t.Fatalf("fixed-step slide = %+v, split = %+v, grounded = %v", position, split.Position(), together.Grounded())
	}

	for range 120 {
		together.Advance(&world, CharacterStep, v(4, 0, 2), 1)
	}

	position = together.Position()
	if !closeTo(position.X, -.4) || !closeTo(position.Z, 3) {
		t.Fatalf("sustained wall slide = %+v", position)
	}

	before := together.Position()
	together.Advance(&world, time.Second, v(-2, 0, 0), 1)

	if position := together.Position(); !closeTo(position.X, before.X-float32(2*(maxSteps*CharacterStep).Seconds())) {
		t.Fatalf("catch-up cap or departure from wall = %+v", position)
	}
}

//nolint:paralleltest // Go 1.27 requires allocation measurements to run serially.
func TestQueriesDoNotAllocate(t *testing.T) {
	var world World

	world.AddMesh(floor(), 1)
	world.AddMesh(wallX(), 1)
	world.AddTrigger(Bounds{Min: v(-1, 0, -1), Max: v(1, 2, 1)}, 2)

	shape := Capsule{Radius: .4, HalfSegment: .5}
	player := NewCharacter(shape, v(-2, 1, 0))

	var buffer [1]ID

	if allocs := testing.AllocsPerRun(20, func() {
		world.Raycast(v(-2, 1, 0), v(3, 0, 0), 1)
		world.CapsuleCast(v(-2, 1, 0), shape, v(3, 0, 0), 1)
		world.AppendTriggers(buffer[:0], v(0, 1, 0), shape, 2)
		player.Advance(&world, CharacterStep, v(1, 0, 0), 1)
	}); allocs != 0 {
		t.Fatalf("steady collision queries and controller allocated %g objects", allocs)
	}
}

func closeTo(a, b float32) bool { return math.Abs(float64(a-b)) < 1e-4 }
