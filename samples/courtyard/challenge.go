package main

import (
	"slices"
	"time"

	"github.com/karty-game/ashura/pkg/collision"
	"github.com/karty-game/ashura/pkg/engine"
	"github.com/karty-game/ashura/samples/courtyard/layout"
)

type trialStatus struct {
	elapsed, best, gateTime time.Duration
	instruction             string
	route, collected        int
	switchReady, finished   bool
}

func (scene *courtyard) reset(nextRoute bool) {
	if nextRoute {
		scene.route = (scene.route + 1) % routeCount
	}

	scene.player = collision.NewCharacter(walkingShape, engine.Vector3{Y: 1, Z: layout.SpawnZ})
	scene.camera.Rotation = engine.Vector3{X: startPitch}
	scene.syncView()
	scene.elapsed, scene.gateTime, scene.collected = 0, 0, 0
	scene.started, scene.finished, scene.switchReady = false, false, false
	scene.first.setOpen(&scene.queries, false)
	scene.timed.setOpen(&scene.queries, false)
	scene.switchColor.Color, scene.exitColor.Color = cyan, cyan

	for route := range scene.relics {
		for index := range scene.relics[route] {
			item := &scene.relics[route][index]
			item.taken = false
			item.pose.Position.Y = hiddenY

			if route == scene.route {
				item.pose.Position.Y = item.height
			}
		}
	}

	for _, surface := range scene.plateColors {
		surface.Color = violet
	}
}

func (scene *courtyard) update(input courtyardInput, elapsed time.Duration) {
	if input.restart {
		scene.reset(scene.finished)

		return
	}

	rotation := &scene.camera.Rotation
	rotation.Y -= input.look.X * lookSpeed
	rotation.X = max(-maxPitch, min(maxPitch, rotation.X-input.look.Y*lookSpeed))

	if !scene.finished {
		scene.started = scene.started || input.move != (engine.Vector2{})

		if scene.started {
			scene.elapsed += elapsed
		}
	}

	scene.player.Advance(&scene.queries, elapsed, walkVelocity(input.move, rotation.Y), solid)
	scene.syncView()
	scene.updateSwitch(input.use)

	if !scene.finished {
		scene.updateTriggers(elapsed)
	}
}

func (scene *courtyard) syncView() {
	scene.camera.Position = scene.player.ViewPosition()
	scene.camera.Position.Y += eyeOffset
}

func (scene *courtyard) updateSwitch(activate bool) {
	if scene.first.id == 0 {
		return
	}

	hit, found := scene.queries.Raycast(scene.camera.Position, scene.rayDirection(), solid|switchTarget)
	scene.switchReady = found && hit.ID == scene.switchID
	scene.switchColor.Color = cyan

	if scene.switchReady {
		scene.switchColor.Color = cyanLit

		if activate {
			scene.first.setOpen(&scene.queries, true)
			scene.switchReady = false
		}
	}
}

func (scene *courtyard) updateTriggers(elapsed time.Duration) {
	hits := scene.queries.AppendTriggers(scene.overlaps[:0], scene.player.Position(), walkingShape, trigger)
	scene.updateTimedGate(hits, elapsed)

	for index := range scene.relics[scene.route] {
		item := &scene.relics[scene.route][index]
		if !item.taken && slices.Contains(hits, item.overlap) {
			item.taken = true
			item.pose.Position.Y = hiddenY
			scene.collected++
		}
	}

	if scene.collected == relicCount {
		scene.exitColor.Color = green
		if slices.Contains(hits, scene.exit) {
			scene.finish()
		}
	}
}

func (scene *courtyard) updateTimedGate(hits []collision.ID, elapsed time.Duration) {
	held := false

	for index, id := range scene.plateIDs {
		color := violet

		if slices.Contains(hits, id) {
			held = true
			color = cyanLit
		}

		scene.plateColors[index].Color = color
	}

	scene.gateTime = max(0, scene.gateTime-elapsed)
	if held {
		scene.gateTime = gateWindow
	}

	// A gate whose timer expires while the capsule is in the doorway waits
	// until the player clears it before restoring the solid collider.
	open := scene.gateTime > 0 || scene.timed.id == 0 && slices.Contains(hits, scene.doorway)
	scene.timed.setOpen(&scene.queries, open)
}

func (scene *courtyard) finish() {
	scene.finished = true
	scene.gateTime = 0
	best := &scene.best[scene.route]

	if *best == 0 || scene.elapsed < *best {
		*best = scene.elapsed
	}
}

func (scene *courtyard) status() trialStatus {
	var instruction string

	switch {
	case scene.finished:
		instruction = "Run complete. Try the other route."
	case scene.first.id != 0 && scene.collected == 0:
		instruction = "Collect the gold relic and aim at the cyan switch. Use it to lift the copper gate."
	case scene.first.id != 0:
		instruction = "Aim at the cyan switch and use it to lift the copper gate."
	case scene.collected == relicCount:
		instruction = "All relics collected. Step on the green exit."
	case scene.gateTime > 0:
		instruction = "The violet gate is open. Go through before it closes."
	default:
		instruction = "Collect the gold relics. Step on a violet plate to lift the timed gate."
	}

	return trialStatus{
		elapsed: scene.elapsed, best: scene.best[scene.route], gateTime: scene.gateTime,
		route: scene.route, collected: scene.collected, instruction: instruction,
		switchReady: scene.switchReady, finished: scene.finished,
	}
}
