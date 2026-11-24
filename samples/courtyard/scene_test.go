package main

import (
	"math"
	"testing"
	"time"

	"github.com/karty-game/ashura/pkg/collision"
	"github.com/karty-game/ashura/pkg/engine"
	"github.com/karty-game/ashura/samples/courtyard/layout"
)

func TestCourtyardSwitchAndTimedPlate(t *testing.T) {
	t.Parallel()

	scene, err := buildCourtyard(courtyardArt{})
	if err != nil {
		t.Fatal(err)
	}

	scene.update(courtyardInput{use: true}, 0)

	if scene.switchReady || scene.first.id == 0 {
		t.Fatal("switch activated from spawn")
	}

	walking := courtyardInput{move: stickMovement(engine.Vector2{Y: -stickRadius})}
	for range 90 {
		scene.update(walking, collision.CharacterStep)
	}

	if position := scene.player.Position(); position.Z < 3.49 || position.Z > 3.51 || !scene.player.Grounded() {
		t.Fatalf("approach did not stop at the copper gate: %+v", position)
	}

	setPose(scene, engine.Vector3{X: -1.85, Y: .85, Z: 5.2})
	scene.update(courtyardInput{look: engine.Vector2{X: 450}, use: true}, 0)

	if scene.switchReady || scene.first.id == 0 {
		t.Fatal("switch activated while looking away")
	}

	scene.update(courtyardInput{look: engine.Vector2{X: -450}}, 0)

	if !scene.switchReady {
		t.Fatal("ray did not select the switch")
	}

	scene.update(courtyardInput{use: true}, 0)

	if scene.first.id != 0 || scene.switchReady {
		t.Fatal("switch did not open the copper gate")
	}

	pressPlate(scene, 0)

	if scene.timed.id != 0 || scene.gateTime != gateWindow {
		t.Fatal("plate did not open timed gate")
	}

	setPose(scene, engine.Vector3{Y: .85, Z: -4.5})
	scene.update(courtyardInput{}, gateWindow)

	if scene.timed.id != 0 || scene.gateTime != 0 {
		t.Fatal("gate closed through the player")
	}

	setPose(scene, engine.Vector3{Y: .85, Z: -5.9})
	scene.update(courtyardInput{}, collision.CharacterStep)

	if scene.timed.id == 0 {
		t.Fatal("gate did not close after player cleared doorway")
	}

	pressPlate(scene, 1)

	if scene.timed.id != 0 {
		t.Fatal("return plate did not reopen gate")
	}
}

func TestCourtyardRelicsAndReplay(t *testing.T) {
	t.Parallel()

	scene, err := buildCourtyard(courtyardArt{})
	if err != nil {
		t.Fatal(err)
	}

	scene.update(courtyardInput{move: engine.Vector2{Y: 1}}, collision.CharacterStep)

	touchFloor(scene, layout.Finish())

	if scene.finished {
		t.Fatal("exit accepted an incomplete relic set")
	}

	takeRoute(scene, 1)

	if scene.collected != 0 {
		t.Fatal("another route's relics counted")
	}

	takeRoute(scene, 0)
	takeRoute(scene, 0)

	if scene.collected != relicCount || scene.exitColor.Color != green {
		t.Fatal("each relic should count once and unlock the exit")
	}

	touchFloor(scene, layout.Finish())

	if !scene.finished || scene.best[0] != scene.elapsed || scene.best[0] == 0 {
		t.Fatal("finish did not freeze and record first route time")
	}

	best := scene.best[0]
	scene.update(courtyardInput{restart: true}, 0)

	if scene.route != 1 || scene.collected != 0 || scene.elapsed != 0 || scene.best[0] != best ||
		scene.timed.id == 0 || scene.first.id == 0 || scene.relics[1][0].pose.Position.Y == hiddenY {
		t.Fatal("next route did not restore the course and preserve the record")
	}

	scene.update(courtyardInput{restart: true}, 0)

	if scene.route != 1 {
		t.Fatal("restart should repeat an unfinished route")
	}
}

func setPose(scene *courtyard, center engine.Vector3) {
	scene.player = collision.NewCharacter(walkingShape, center)
	scene.camera.Rotation = engine.Vector3{X: startPitch}
	scene.syncView()
}

func touchFloor(scene *courtyard, floor collision.Bounds) {
	setPose(scene, engine.Vector3{X: (floor.Min.X + floor.Max.X) * boxMiddle, Y: .85, Z: (floor.Min.Z + floor.Max.Z) * boxMiddle})
	scene.update(courtyardInput{}, 0)
}

func pressPlate(scene *courtyard, index int) { touchFloor(scene, layout.Plates()[index]) }

func takeRoute(scene *courtyard, route int) {
	for _, position := range layout.Relics()[route] {
		position.Y = .85
		setPose(scene, position)
		scene.update(courtyardInput{}, 0)
	}
}

func TestStickKeepsDiagonalSpeedBounded(t *testing.T) {
	t.Parallel()

	velocity := walkVelocity(stickMovement(engine.Vector2{X: stickRadius, Y: -stickRadius}), 0)
	if math.Abs(math.Hypot(float64(velocity.X), float64(velocity.Z))-walkSpeed) > 1e-6 {
		t.Fatalf("diagonal speed = %+v", velocity)
	}
}

func TestResetPreservesBestAndAlternatesRoute(t *testing.T) {
	t.Parallel()

	scene, err := buildCourtyard(courtyardArt{})
	if err != nil {
		t.Fatal(err)
	}

	scene.elapsed = 5 * time.Second
	scene.finish()
	scene.reset(scene.finished)
	scene.elapsed = 6 * time.Second
	scene.finish()
	scene.reset(scene.finished)
	scene.elapsed = 7 * time.Second
	scene.finish()

	if scene.route != 0 || scene.best != [routeCount]time.Duration{5 * time.Second, 6 * time.Second} {
		t.Fatalf("per-route times = %v, route = %d", scene.best, scene.route)
	}
}

func TestCourtyardGateArtClearsFrame(t *testing.T) {
	t.Parallel()

	scene, err := buildCourtyard(courtyardArt{})
	if err != nil {
		t.Fatal(err)
	}

	for _, fixture := range [...]struct {
		name    string
		opening collision.Bounds
		door    *gate
	}{
		{"copper", layout.FirstGate(), &scene.first},
		{"violet", layout.TimedGate(), &scene.timed},
	} {
		panel := gatePanelBounds(fixture.opening)
		posts, header := gateFrameBounds(fixture.opening)
		floorY := layout.Floor().Max.Y

		if panel.Min.Y <= floorY || panel.Min.X <= fixture.opening.Min.X || panel.Max.X >= fixture.opening.Max.X ||
			posts[0].Max.X >= fixture.opening.Min.X || posts[1].Min.X <= fixture.opening.Max.X {
			t.Fatalf("%s gate panel or jambs share a face with the opening", fixture.name)
		}

		for _, post := range posts {
			if post.Min.Y <= floorY || post.Max.Y >= header.Min.Y {
				t.Fatalf("%s jamb touches the floor or lintel", fixture.name)
			}
		}

		fixture.door.setOpen(&scene.queries, true)

		raisedBottom := panel.Min.Y + fixture.door.pose.Position.Y - fixture.door.restY
		if raisedBottom <= header.Max.Y {
			t.Fatalf("%s open panel bottom %f intersects lintel top %f", fixture.name, raisedBottom, header.Max.Y)
		}
	}
}
