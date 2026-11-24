package main

import (
	"math"
	"time"

	"github.com/karty-game/ashura/pkg/debug"
	"github.com/karty-game/ashura/pkg/engine"
)

const (
	stickRadius   = 36 // CSS pixels
	stickDeadZone = .12
	clockTick     = 100 * time.Millisecond
)

type courtyardInput struct {
	move, look   engine.Vector2
	use, restart bool
}

func (scene *courtyard) bindControls(app *engine.App, canvasID string, panel *debug.Panel) func() {
	var (
		touch       courtyardInput
		spaceHeld   bool
		restartHeld bool
	)

	present, cleanup := attachTouchControls(canvasID, &touch)

	app.OnUpdate(func(elapsed time.Duration) {
		input := app.Input()
		frame := touch
		touch.use, touch.restart = false, false
		frame.move.X += axis(input, engine.KeyD, engine.KeyA)
		frame.move.Y += axis(input, engine.KeyW, engine.KeyS)
		frame.look = input.TouchDelta()

		if input.MouseButtonDown(engine.MouseButtonLeft) {
			mouse := input.MouseDelta()
			frame.look.X += mouse.X
			frame.look.Y += mouse.Y
		}

		space := input.KeyDown(engine.KeySpace)
		frame.use = frame.use || space && !spaceHeld
		spaceHeld = space

		restart := input.KeyDown(engine.KeyR)
		frame.restart = frame.restart || restart && !restartHeld
		restartHeld = restart

		scene.update(frame, elapsed)
		present(scene.status(), panel.Active())
	})

	return cleanup
}

func walkVelocity(move engine.Vector2, yaw float32) engine.Vector3 {
	strafe, forward := float64(move.X), float64(move.Y)
	sinYaw, cosYaw := math.Sincos(float64(yaw))
	speed := walkSpeed / max(1, math.Hypot(strafe, forward))

	return engine.Vector3{X: float32((strafe*cosYaw - forward*sinYaw) * speed), Z: float32((-forward*cosYaw - strafe*sinYaw) * speed)}
}

func axis(input engine.Input, positive, negative engine.Key) float32 {
	var value float32
	if input.KeyDown(positive) {
		value++
	}

	if input.KeyDown(negative) {
		value--
	}

	return value
}

func stickMovement(offset engine.Vector2) engine.Vector2 {
	strafe, forward := float64(offset.X)/stickRadius, -float64(offset.Y)/stickRadius

	distance := math.Hypot(strafe, forward)
	if distance <= stickDeadZone {
		return engine.Vector2{}
	}

	scale := (min(distance, 1) - stickDeadZone) / ((1 - stickDeadZone) * distance)

	return engine.Vector2{X: float32(strafe * scale), Y: float32(forward * scale)}
}
