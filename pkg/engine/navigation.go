package engine

import (
	"math"
	"time"

	"github.com/karty-game/ashura/internal/platform"
)

const (
	defaultOrbitDistance    float32 = 8
	minimumCameraDistance   float32 = .1
	maximumCameraDistance   float32 = 1000
	defaultCameraZoomSpeed  float32 = .1
	defaultPanSpeed         float32 = 1
	defaultMaximumMapHeight float32 = 100
	maximumZoomExponent             = 20
	maximumCameraFOV                = 180
)

type orbitControlsComponent interface {
	RuntimeOrbitControls() (*Vector3, *float32, float32, float32, float32, float32, MouseButton)
}

type panZoomControlsComponent interface {
	RuntimePanZoomControls() (float32, float32, float32, float32, MouseButton)
}

type viewHeightComponent interface{ RuntimeViewHeight() *float32 }

type orbitSettings struct {
	target                                   *Vector3
	distance                                 *float32
	minimum, maximum, rotateSpeed, zoomSpeed float32
	button                                   MouseButton
	disableTouchRotate                       bool
}

type panSettings struct {
	speed, zoomSpeed, minimum, maximum float32
	button                             MouseButton
}

func updateCameraControls(world *World, delta time.Duration, input platform.InputState) {
	entity, found := activeCameraEntity(world)
	if !found {
		return
	}

	transform := world.Transform(entity)
	if transform == nil {
		return
	}

	if settings, ok := orbitControlSettings(world, entity); ok {
		updateOrbit(world, entity, transform, settings, input)
	} else if settings, ok := panControlSettings(world, entity); ok {
		updatePanZoom(world, entity, transform, settings, input)
	} else {
		updateFreeCameraEntity(world, entity, delta, input)
	}
}

func orbitControlSettings(world *World, entity Entity) (orbitSettings, bool) {
	var settings orbitSettings
	if world == nil {
		return settings, false
	}

	entry := world.bindings[entity]
	if entry == nil || entry.orbit == nil {
		return settings, false
	}

	settings.target, settings.distance, settings.minimum, settings.maximum, settings.rotateSpeed, settings.zoomSpeed, settings.button = entry.orbit.RuntimeOrbitControls()

	if touch, ok := entry.orbit.(interface{ RuntimeOrbitTouchRotation() bool }); ok {
		settings.disableTouchRotate = !touch.RuntimeOrbitTouchRotation()
	}

	return settings, settings.target != nil && settings.distance != nil
}

func panControlSettings(world *World, entity Entity) (panSettings, bool) {
	var settings panSettings
	if world == nil {
		return settings, false
	}

	entry := world.bindings[entity]
	if entry == nil || entry.pan == nil {
		return settings, false
	}

	settings.speed, settings.zoomSpeed, settings.minimum, settings.maximum, settings.button = entry.pan.RuntimePanZoomControls()

	return settings, true
}

func viewHeightForEntity(world *World, entity Entity) *float32 {
	if world == nil {
		return nil
	}

	entry := world.bindings[entity]
	if entry == nil || entry.height == nil {
		return nil
	}

	return entry.height.RuntimeViewHeight()
}

func updateOrbit(world *World, entity Entity, transform *Transform, settings orbitSettings, input platform.InputState) {
	initializeOrbit(transform, settings)
	minimum, maximum := cameraRange(settings.minimum, settings.maximum, maximumCameraDistance)

	deltaX, deltaY := mouseDragDelta(input, settings.button)
	if !settings.disableTouchRotate {
		deltaX += input.TouchDX
		deltaY += input.TouchDY
	}

	rotateCamera(transform, deltaX, deltaY, positiveOr(settings.rotateSpeed, defaultFreeLookSensitivity))
	transform.Rotation.Z = 0

	height := viewHeightForEntity(world, entity)
	if height != nil {
		*height = zoomExtent(positiveOr(*height, defaultViewHeight), minimum, maximum, settings.zoomSpeed, input)
	} else {
		*settings.distance = zoomExtent(*settings.distance, minimum, maximum, settings.zoomSpeed, input)
	}

	if input.TouchPanX != 0 || input.TouchPanY != 0 {
		viewSpan := orbitPanSpan(world, entity, height, *settings.distance)
		offset := panOffset(transform, input.TouchPanX, input.TouchPanY, viewSpan/logicalHeight(input))
		settings.target.X += offset.X
		settings.target.Y += offset.Y
		settings.target.Z += offset.Z
	}

	forward := cameraForward(transform)
	distance := *settings.distance
	transform.Position = Vector3{
		X: settings.target.X - forward.X*distance,
		Y: settings.target.Y - forward.Y*distance,
		Z: settings.target.Z - forward.Z*distance,
	}
}

func orbitPanSpan(world *World, entity Entity, height *float32, distance float32) float32 {
	if height != nil {
		return *height
	}

	projection, _ := projectionForEntity(world, entity)

	fov := projection.fieldOfViewDegrees
	if fov <= 0 || fov >= maximumCameraFOV {
		fov = defaultFOV
	}

	const diameter = 2

	return diameter * distance * float32(math.Tan(float64(fov*degreesToRadians)/diameter))
}

func initializeOrbit(transform *Transform, settings orbitSettings) {
	if *settings.distance > 0 {
		return
	}

	offset := Vector3{
		X: transform.Position.X - settings.target.X,
		Y: transform.Position.Y - settings.target.Y,
		Z: transform.Position.Z - settings.target.Z,
	}

	distance := float32(math.Sqrt(float64(offset.X*offset.X + offset.Y*offset.Y + offset.Z*offset.Z)))
	if distance > 0 {
		*settings.distance = distance
		transform.Rotation.X = -float32(math.Asin(float64(clamp(offset.Y/distance, -1, 1))))
		transform.Rotation.Y = float32(math.Atan2(float64(offset.X), float64(offset.Z)))
	} else {
		*settings.distance = defaultOrbitDistance
	}
}

func updatePanZoom(world *World, entity Entity, transform *Transform, settings panSettings, input platform.InputState) {
	height := viewHeightForEntity(world, entity)
	if height == nil {
		return
	}

	minimum, maximum := cameraRange(settings.minimum, settings.maximum, defaultMaximumMapHeight)
	*height = zoomExtent(positiveOr(*height, defaultViewHeight), minimum, maximum, settings.zoomSpeed, input)
	deltaX, deltaY := mouseDragDelta(input, settings.button)
	deltaX += input.TouchDX + input.TouchPanX
	deltaY += input.TouchDY + input.TouchPanY
	panCamera(transform, deltaX, deltaY, *height*positiveOr(settings.speed, defaultPanSpeed)/logicalHeight(input))
}

func mouseDragDelta(input platform.InputState, button MouseButton) (float32, float32) {
	if int(button) < len(input.DragDX) {
		return input.DragDX[button], input.DragDY[button]
	}

	return 0, 0
}

func rotateCamera(transform *Transform, deltaX, deltaY, speed float32) {
	if deltaX == 0 && deltaY == 0 {
		return
	}

	sensitivity := speed * degreesToRadians
	transform.Rotation.Y -= deltaX * sensitivity
	transform.Rotation.X = clamp(transform.Rotation.X-deltaY*sensitivity, -maximumCameraPitch, maximumCameraPitch)
}

func cameraForward(transform *Transform) Vector3 {
	rotation := transformMatrix(Transform{Rotation: transform.Rotation, Scale: Vector3One()})

	return Vector3{X: -rotation[8], Y: -rotation[9], Z: -rotation[10]}
}

func panOffset(transform *Transform, deltaX, deltaY, scale float32) Vector3 {
	if deltaX == 0 && deltaY == 0 {
		return Vector3{}
	}

	rotation := transformMatrix(Transform{Rotation: transform.Rotation, Scale: Vector3One()})

	return Vector3{
		X: (-rotation[0]*deltaX + rotation[4]*deltaY) * scale,
		Y: (-rotation[1]*deltaX + rotation[5]*deltaY) * scale,
		Z: (-rotation[2]*deltaX + rotation[6]*deltaY) * scale,
	}
}

func panCamera(transform *Transform, deltaX, deltaY, scale float32) {
	offset := panOffset(transform, deltaX, deltaY, scale)
	transform.Position.X += offset.X
	transform.Position.Y += offset.Y
	transform.Position.Z += offset.Z
}

func logicalHeight(input platform.InputState) float32 {
	if input.LogicalHeight > 0 {
		return input.LogicalHeight
	}

	return positiveOr(float32(input.Height), defaultViewHeight)
}

func positiveOr(value, fallback float32) float32 {
	if value > 0 {
		return value
	}

	return fallback
}

func cameraRange(minimum, maximum, fallbackMaximum float32) (float32, float32) {
	minimum = positiveOr(minimum, minimumCameraDistance)
	maximum = positiveOr(maximum, fallbackMaximum)

	return minimum, max(minimum, maximum)
}

func zoomExtent(value, minimum, maximum, speed float32, input platform.InputState) float32 {
	exponent := positiveOr(speed, defaultCameraZoomSpeed)*input.WheelY + input.Pinch
	if exponent == 0 {
		return clamp(value, minimum, maximum)
	}

	return clamp(value*float32(math.Exp(float64(clamp(exponent, -maximumZoomExponent, maximumZoomExponent)))), minimum, maximum)
}
