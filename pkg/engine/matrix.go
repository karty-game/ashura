package engine

import (
	"math"
)

const (
	clipDefaultNear    float32 = 0.1
	clipDefaultFar     float32 = 1000
	defaultFOV                 = 60
	defaultViewHeight  float32 = 20
	degreesPerHalfTurn         = 360
	matrixDimension            = 4
	projectionScale    float32 = 2
)

type matrix4 [16]float32

func transformMatrix(transform Transform) [16]float32 {
	rx := rotationX(transform.Rotation.X)
	ry := rotationY(transform.Rotation.Y)
	rz := rotationZ(transform.Rotation.Z)
	result := multiply(multiply(rz, ry), rx)
	result[0] *= transform.Scale.X
	result[1] *= transform.Scale.X
	result[2] *= transform.Scale.X
	result[4] *= transform.Scale.Y
	result[5] *= transform.Scale.Y
	result[6] *= transform.Scale.Y
	result[8] *= transform.Scale.Z
	result[9] *= transform.Scale.Z
	result[10] *= transform.Scale.Z
	result[12], result[13], result[14] = transform.Position.X, transform.Position.Y, transform.Position.Z
	result[15] = 1

	return [16]float32(result)
}

func perspectiveViewProjection(transform Transform, projection projectionSettings, aspect float32) [16]float32 {
	fieldOfView := projection.fieldOfViewDegrees
	if fieldOfView <= 0 || fieldOfView >= 180 {
		fieldOfView = defaultFOV
	}

	near, far := clipPlanes(projection.near, projection.far)
	f := 1 / float32(math.Tan(float64(fieldOfView)*math.Pi/degreesPerHalfTurn))
	project := matrix4{}
	project[0], project[5] = f/aspect, f
	project[10] = far / (near - far)
	project[11] = -1
	project[14] = near * far / (near - far)

	return [16]float32(multiply(project, viewMatrix(transform)))
}

func orthographicViewProjection(transform Transform, projection projectionSettings, aspect float32) [16]float32 {
	viewHeight := projection.viewHeight
	if viewHeight <= 0 {
		viewHeight = defaultViewHeight
	}

	near, far := clipPlanes(projection.near, projection.far)
	viewWidth := viewHeight * aspect
	project := matrix4{
		0:  projectionScale / viewWidth,
		5:  projectionScale / viewHeight,
		10: 1 / (near - far),
		14: near / (near - far),
		15: 1,
	}

	return [16]float32(multiply(project, viewMatrix(transform)))
}

func cameraViewProjection(transform Transform, projection projectionSettings, aspect float32) [16]float32 {
	if projectionIsPerspective(projection) {
		return perspectiveViewProjection(transform, projection, aspect)
	}

	return orthographicViewProjection(transform, projection, aspect)
}

func clipPlanes(near, far float32) (float32, float32) {
	if near <= 0 {
		near = clipDefaultNear
	}

	if far <= near {
		far = clipDefaultFar
		if far <= near {
			far = near + clipDefaultFar
		}
	}

	return near, far
}

func viewMatrix(transform Transform) matrix4 {
	rotation := matrix4(transformMatrix(Transform{
		Rotation: transform.Rotation,
		Scale:    Vector3One(),
	}))
	view := matrix4{}

	for row := range 3 {
		for column := range 3 {
			view[column*4+row] = rotation[row*4+column]
		}
	}

	view[15] = 1
	view[12] = -(view[0]*transform.Position.X + view[4]*transform.Position.Y + view[8]*transform.Position.Z)
	view[13] = -(view[1]*transform.Position.X + view[5]*transform.Position.Y + view[9]*transform.Position.Z)
	view[14] = -(view[2]*transform.Position.X + view[6]*transform.Position.Y + view[10]*transform.Position.Z)

	return view
}

func rotationX(angle float32) matrix4 {
	c, s := float32(math.Cos(float64(angle))), float32(math.Sin(float64(angle)))

	return matrix4{0: 1, 5: c, 6: s, 9: -s, 10: c, 15: 1}
}

func rotationY(angle float32) matrix4 {
	c, s := float32(math.Cos(float64(angle))), float32(math.Sin(float64(angle)))

	return matrix4{0: c, 2: -s, 5: 1, 8: s, 10: c, 15: 1}
}

func rotationZ(angle float32) matrix4 {
	c, s := float32(math.Cos(float64(angle))), float32(math.Sin(float64(angle)))

	return matrix4{0: c, 1: s, 4: -s, 5: c, 10: 1, 15: 1}
}

func multiply(left, right matrix4) matrix4 {
	var result matrix4

	for column := range matrixDimension {
		for row := range matrixDimension {
			for inner := range matrixDimension {
				result[column*4+row] += left[inner*4+row] * right[column*4+inner]
			}
		}
	}

	return result
}
