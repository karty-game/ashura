package main

import (
	"math"

	"github.com/karty-game/karty-webgpu/pkg/engine"
)

const fullTurn = 2 * math.Pi

type rotationMode uint8

const (
	rotationPaused rotationMode = iota
	rotationYaw
	rotationPitch
	rotationRoll
	rotationModeCount
)

func (mode rotationMode) label() string {
	switch mode {
	case rotationYaw:
		return "yaw"
	case rotationPitch:
		return "pitch"
	case rotationRoll:
		return "roll"
	case rotationPaused, rotationModeCount:
		return "paused"
	}

	return "paused"
}

func advanceRotation(rotation engine.Vector3, mode rotationMode, step float32) engine.Vector3 {
	var angle *float32

	switch mode {
	case rotationYaw:
		angle = &rotation.Y
	case rotationPitch:
		angle = &rotation.X
	case rotationRoll:
		angle = &rotation.Z
	case rotationPaused, rotationModeCount:
		return rotation
	}

	if angle != nil {
		*angle += step
		if *angle >= fullTurn {
			*angle -= fullTurn
		}
	}

	return rotation
}

func (comparison *comparisonScene) applyPose() {
	rotation := comparison.state.rotation
	rotatedCenter := rotateCenter(comparison.center, rotation)

	translation := engine.Vector3{
		X: comparison.center.X - rotatedCenter.X,
		Y: comparison.center.Y - rotatedCenter.Y,
		Z: comparison.center.Z - rotatedCenter.Z,
	}
	for _, transform := range comparison.modelTransforms {
		transform.Position = translation
		transform.Position.X -= pairOffset
		transform.Rotation = rotation
	}

	if comparison.impostorTransform != nil {
		comparison.impostorTransform.Position = translation
		comparison.impostorTransform.Position.X += pairOffset
		comparison.impostorTransform.Rotation = rotation
	}
}

// rotateCenter follows the engine's Rz * Ry * Rx Euler order, keeping both
// prepared representations centered while their shared root orientation changes.
func rotateCenter(center, rotation engine.Vector3) engine.Vector3 {
	sinX, cosX := math.Sincos(float64(rotation.X))
	sinY, cosY := math.Sincos(float64(rotation.Y))
	sinZ, cosZ := math.Sincos(float64(rotation.Z))
	firstX, firstY, firstZ := float64(
		center.X,
	), cosX*float64(
		center.Y,
	)-sinX*float64(
		center.Z,
	), sinX*float64(
		center.Y,
	)+cosX*float64(
		center.Z,
	)
	secondX, secondZ := cosY*firstX+sinY*firstZ, -sinY*firstX+cosY*firstZ

	return engine.Vector3{X: float32(cosZ*secondX - sinZ*firstY), Y: float32(sinZ*secondX + cosZ*firstY), Z: float32(secondZ)}
}
