package particledata

import (
	"encoding/binary"
	"math"
)

// CameraBytes holds a view-projection matrix and two padded camera axes.
const (
	CameraBytes       = 96
	CameraGroup       = 2
	cameraRightOffset = 64
	cameraUpOffset    = 80
	wgslFloatSize     = 4
)

// Camera caches the GPU billboard transform until the active camera changes.
type Camera struct {
	matrix [16]float32
	Bytes  [CameraBytes]byte
}

func (camera *Camera) Update(matrix [16]float32) bool {
	if matrix == camera.matrix {
		return false
	}

	camera.matrix = matrix
	for index, value := range matrix {
		binary.LittleEndian.PutUint32(camera.Bytes[index*wgslFloatSize:], math.Float32bits(value))
	}

	// The XYZ parts of the first two projection rows are the scaled view axes.
	camera.packAxis(cameraRightOffset, [3]float32{matrix[0], matrix[4], matrix[8]})
	camera.packAxis(cameraUpOffset, [3]float32{matrix[1], matrix[5], matrix[9]})

	return true
}

func (camera *Camera) packAxis(offset int, axis [3]float32) {
	var sum float64
	for _, component := range axis {
		sum += float64(component) * float64(component)
	}

	length := math.Sqrt(sum)
	if length == 0 {
		return
	}

	for index, value := range axis {
		binary.LittleEndian.PutUint32(camera.Bytes[offset+index*wgslFloatSize:], math.Float32bits(float32(float64(value)/length)))
	}
}
