package engine

import (
	"math"
	"time"

	"github.com/karty-game/ashura/internal/platform"
)

const (
	defaultFreeMoveSpeed       float32 = 5
	defaultFreeLookSensitivity float32 = 0.2
	degreesToRadians                   = float32(math.Pi / 180)
	maximumCameraPitch                 = float32(math.Pi/2 - 0.01)
)

type freeControlsComponent interface {
	RuntimeFreeControls() (moveSpeed, lookSensitivity float32)
}

func updateFreeCameraControls(world *World, delta time.Duration, input platform.InputState) {
	entity, found := activeCameraEntity(world)
	if !found {
		return
	}

	updateFreeCameraEntity(world, entity, delta, input)
}

func updateFreeCameraEntity(world *World, entity Entity, delta time.Duration, input platform.InputState) {
	moveSpeed, lookSensitivity, found := freeControlSettings(world, entity)
	if !found {
		return
	}

	transform := world.Transform(entity)
	if transform == nil {
		return
	}

	if moveSpeed <= 0 {
		moveSpeed = defaultFreeMoveSpeed
	}

	if lookSensitivity <= 0 {
		lookSensitivity = defaultFreeLookSensitivity
	}

	deltaX, deltaY := input.TouchDX, input.TouchDY
	if input.PointerLocked {
		deltaX += input.MouseDX
		deltaY += input.MouseDY
	} else {
		deltaX += input.DragDX[MouseButtonLeft]
		deltaY += input.DragDY[MouseButtonLeft]
	}

	rotateCamera(transform, deltaX, deltaY, lookSensitivity)
	panCamera(transform, input.TouchPanX, input.TouchPanY, moveSpeed/logicalHeight(input))

	if input.Pinch != 0 {
		forward := cameraForward(transform)
		distance := -input.Pinch * moveSpeed
		transform.Position.X += forward.X * distance
		transform.Position.Y += forward.Y * distance
		transform.Position.Z += forward.Z * distance
	}

	moveX := keyAxis(input.Keys, platform.KeyCodeD, platform.KeyCodeA)
	moveZ := keyAxis(input.Keys, platform.KeyCodeW, platform.KeyCodeS)
	moveY := keyAxis(input.Keys, platform.KeyCodeSpace, platform.KeyCodeShift)
	rotation := float64(transform.Rotation.Y)
	forwardX, forwardZ := float32(-math.Sin(rotation)), float32(-math.Cos(rotation))
	rightX, rightZ := float32(math.Cos(rotation)), float32(-math.Sin(rotation))
	velocityX := forwardX*moveZ + rightX*moveX
	velocityZ := forwardZ*moveZ + rightZ*moveX

	magnitude := float32(math.Sqrt(float64(velocityX*velocityX + velocityZ*velocityZ + moveY*moveY)))
	if magnitude > 1 {
		velocityX /= magnitude
		velocityZ /= magnitude
		moveY /= magnitude
	}

	distance := moveSpeed * float32(delta.Seconds())
	transform.Position.X += velocityX * distance
	transform.Position.Y += moveY * distance
	transform.Position.Z += velocityZ * distance
}

func activeCameraEntity(world *World) (Entity, bool) {
	if entity, found := world.ActiveCamera(); found {
		return entity, true
	}

	if world != nil && len(world.cameras) != 0 {
		return world.cameras[0], true
	}

	return Entity{}, false
}

func freeControlSettings(world *World, entity Entity) (float32, float32, bool) {
	if world == nil {
		return 0, 0, false
	}

	entry := world.bindings[entity]
	if entry == nil || entry.free == nil {
		return 0, 0, false
	}

	speed, sensitivity := entry.free.RuntimeFreeControls()

	return speed, sensitivity, true
}

func keyAxis(keys [256]bool, positive, negative uint8) float32 {
	var value float32
	if keys[positive] {
		value++
	}

	if keys[negative] {
		value--
	}

	return value
}

func clamp(value, minimum, maximum float32) float32 { return max(minimum, min(maximum, value)) }
