package engine

import "math"

const (
	billboardSpherical uint8 = iota
	billboardAxis
	billboardFixed
)

const (
	billboardMinimumLength        = 1e-6
	billboardMinimumLengthSquared = billboardMinimumLength * billboardMinimumLength
	billboardReferenceThreshold   = .5
)

type billboardSnapshot struct {
	transform                      Transform
	cameraPosition, cameraRotation Vector3
	axis                           Vector3
	mode                           uint8
	camera                         bool
}

func (world *World) drawMatrix(entity Entity, transform Transform, billboard bool, provider billboardComponent) [16]float32 {
	batch := world.frame.batch(entity.index)
	if !billboard {
		if !batch.matrixReady || batch.billboardActive || batch.transform != transform {
			batch.transform, batch.matrix = transform, transformMatrix(transform)
			batch.matrixReady = true
			batch.billboardActive = false
		}

		return batch.matrix
	}

	key := billboardSnapshot{transform: transform, camera: world.frame.cameraReady}
	if provider != nil {
		key.mode, key.axis = provider.RuntimeBillboard()
	}

	if key.mode != billboardFixed {
		key.cameraPosition = world.frame.cameraTransform.Position
	} else {
		key.camera = false
	}

	if key.mode == billboardSpherical {
		key.cameraRotation = world.frame.cameraTransform.Rotation
	}

	if !batch.matrixReady || !batch.billboardActive || batch.billboard != key {
		batch.matrix = billboardMatrix(key)
		batch.billboard = key
		batch.billboardActive = true
		batch.matrixReady = true
	}

	return batch.matrix
}

// Billboard quads use +Z as their front and retain authored world-space size.
// Spherical billboards face the camera position with camera-vertical roll. Axis mode
// locks the world-space axis (default +Y). Fixed mode retains authored rotation.
func billboardMatrix(key billboardSnapshot) [16]float32 {
	if !key.camera || key.mode == billboardFixed {
		return transformMatrix(key.transform)
	}

	forward := subtractVector(key.cameraPosition, key.transform.Position)

	var right, vertical Vector3
	if key.mode == billboardAxis {
		vertical = unitVector(key.axis, Vector3Up())

		forward = rejectVector(forward, vertical)
		if vectorLengthSquared(forward) < billboardMinimumLengthSquared {
			authored := transformMatrix(key.transform)
			forward = rejectVector(Vector3{X: authored[8], Y: authored[9], Z: authored[10]}, vertical)
		}

		forward = unitVector(forward, perpendicularVector(vertical))
		right = unitVector(crossVector(vertical, forward), perpendicularVector(vertical))
	} else {
		camera := transformMatrix(Transform{Rotation: key.cameraRotation, Scale: Vector3One()})
		forward = unitVector(forward, Vector3{X: camera[8], Y: camera[9], Z: camera[10]})
		vertical = Vector3{X: camera[4], Y: camera[5], Z: camera[6]}
		right = unitVector(
			crossVector(vertical, forward),
			unitVector(rejectVector(Vector3{X: camera[0], Y: camera[1], Z: camera[2]}, forward), perpendicularVector(forward)),
		)
		vertical = unitVector(crossVector(forward, right), perpendicularVector(forward))
	}

	scale, position := key.transform.Scale, key.transform.Position

	return [16]float32{
		right.X * scale.X,
		right.Y * scale.X,
		right.Z * scale.X,
		0,
		vertical.X * scale.Y,
		vertical.Y * scale.Y,
		vertical.Z * scale.Y,
		0,
		forward.X * scale.Z,
		forward.Y * scale.Z,
		forward.Z * scale.Z,
		0,
		position.X,
		position.Y,
		position.Z,
		1,
	}
}

func subtractVector(a, b Vector3) Vector3 { return Vector3{X: a.X - b.X, Y: a.Y - b.Y, Z: a.Z - b.Z} }
func crossVector(a, b Vector3) Vector3 {
	return Vector3{X: a.Y*b.Z - a.Z*b.Y, Y: a.Z*b.X - a.X*b.Z, Z: a.X*b.Y - a.Y*b.X}
}
func vectorLengthSquared(v Vector3) float64 {
	return float64(v.X)*float64(v.X) + float64(v.Y)*float64(v.Y) + float64(v.Z)*float64(v.Z)
}
func unitVector(value, fallback Vector3) Vector3 {
	length := math.Sqrt(vectorLengthSquared(value))
	if length < billboardMinimumLength || math.IsNaN(length) || math.IsInf(length, 0) {
		return fallback
	}

	return Vector3{X: float32(float64(value.X) / length), Y: float32(float64(value.Y) / length), Z: float32(float64(value.Z) / length)}
}
func rejectVector(value, axis Vector3) Vector3 {
	projection := value.X*axis.X + value.Y*axis.Y + value.Z*axis.Z

	return Vector3{X: value.X - axis.X*projection, Y: value.Y - axis.Y*projection, Z: value.Z - axis.Z*projection}
}
func perpendicularVector(axis Vector3) Vector3 {
	reference := Vector3{X: 1}
	if math.Abs(float64(axis.X)) > billboardReferenceThreshold {
		reference = Vector3Up()
	}

	return unitVector(crossVector(reference, axis), Vector3{Z: 1})
}
