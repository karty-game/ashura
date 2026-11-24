// Package decaldata shares projector preparation between offline tools and the live renderer.
package decaldata

import (
	"encoding/binary"
	"errors"
	"math"
)

const (
	RecordBytes   = 96
	MaskOffset    = 80
	OrderOffset   = 88
	MaxStatic     = 8192
	MaxBatch      = 1024
	MaxCoordinate = 1e12
	Half          = .5
)

var ErrProjector = errors.New("invalid decal projector")
var ErrBatch = errors.New("invalid prepared decal batch")

type Vector [3]float32

type Placement struct {
	Position, Normal, Size Vector
	Min, Max               [2]float32
	Tint                   [4]float32
	Angle                  float32
	Layers                 uint32
	Order                  int32
}

// Projection stores the same axes and image mapping used by the shader.
type Projection struct {
	Right, Up, Depth, Center Vector
	Min, Max                 [2]float32
	Tint                     [4]float32
	Layers                   uint32
	Order                    int32
	Radius                   float32
}

func (placement Placement) Project() (Projection, error) {
	const minimumSize = 1e-5

	for axis, size := range placement.Size {
		if !(size >= minimumSize && size <= MaxCoordinate) || !(abs(placement.Position[axis]) <= MaxCoordinate) {
			return Projection{}, ErrProjector
		}
	}

	if !validInputs(placement) {
		return Projection{}, ErrProjector
	}

	normal := normalize(placement.Normal)
	if normal == (Vector{}) {
		return Projection{}, ErrProjector
	}

	const poleAlignment = .999

	reference := Vector{0, 1, 0}
	if abs(normal[1]) > poleAlignment {
		reference = Vector{0, 0, -1}
	}

	right := normalize(cross(reference, normal))
	up := cross(normal, right)
	sin, cos := math.Sincos(float64(placement.Angle))

	return Projection{
		Right:  rotate(right, up, float32(cos), float32(sin), placement.Size[0]),
		Up:     rotate(up, right, float32(cos), -float32(sin), placement.Size[1]),
		Depth:  rotate(normal, Vector{}, 1, 0, placement.Size[2]),
		Center: placement.Position, Min: placement.Min, Max: placement.Max, Tint: placement.Tint,
		Layers: placement.Layers, Order: placement.Order, Radius: float32(length(placement.Size) * Half),
	}, nil
}

func validInputs(placement Placement) bool {
	if math.IsNaN(float64(placement.Angle)) || math.IsInf(float64(placement.Angle), 0) || placement.Layers == 0 {
		return false
	}

	for axis, low := range placement.Min {
		high := placement.Max[axis]
		if !(low >= 0 && high <= 1 && low < high) {
			return false
		}
	}

	for _, channel := range placement.Tint {
		if !(channel >= 0 && channel <= 1) {
			return false
		}
	}

	return true
}

func (projector Projection) Record() (record [RecordBytes]byte) {
	values := [...]float32{
		projector.Right[0], projector.Right[1], projector.Right[2], projector.Center[0],
		projector.Up[0], projector.Up[1], projector.Up[2], projector.Center[1],
		projector.Depth[0], projector.Depth[1], projector.Depth[2], projector.Center[2],
		projector.Min[0], projector.Min[1], projector.Max[0], projector.Max[1],
		projector.Tint[0], projector.Tint[1], projector.Tint[2], projector.Tint[3],
	}
	for index, value := range values {
		binary.LittleEndian.PutUint32(record[4*index:], math.Float32bits(value))
	}

	binary.LittleEndian.PutUint32(record[MaskOffset:], projector.Layers)
	binary.LittleEndian.PutUint32(record[84:], math.Float32bits(projector.Radius))
	order := uint32(projector.Order) //nolint:gosec // The shader recovers the original signed bits.
	binary.LittleEndian.PutUint32(record[OrderOffset:], order)

	return record
}

func (projector Projection) UV(position, normal Vector) (u, v float32, inside bool) {
	if dot(normal, projector.Depth) <= 0 {
		return 0, 0, false
	}

	delta := Vector{position[0] - projector.Center[0], position[1] - projector.Center[1], position[2] - projector.Center[2]}

	localX, localY, localZ := dot(delta, projector.Right), dot(delta, projector.Up), dot(delta, projector.Depth)
	if max(abs(localX), abs(localY), abs(localZ)) > Half {
		return 0, 0, false
	}

	return projector.Min[0] + (localX+Half)*(projector.Max[0]-projector.Min[0]),
		projector.Min[1] + (Half-localY)*(projector.Max[1]-projector.Min[1]), true
}

func abs(value float32) float32 { return float32(math.Abs(float64(value))) }
func dot(a, b Vector) float32   { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }
func cross(a, b Vector) Vector {
	return Vector{a[1]*b[2] - a[2]*b[1], a[2]*b[0] - a[0]*b[2], a[0]*b[1] - a[1]*b[0]}
}

func length(a Vector) float64 {
	x, y, z := float64(a[0]), float64(a[1]), float64(a[2])

	return math.Sqrt(x*x + y*y + z*z)
}

func normalize(axis Vector) Vector {
	magnitude := length(axis)
	if magnitude < 1e-6 || math.IsNaN(magnitude) || math.IsInf(magnitude, 0) {
		return Vector{}
	}

	return Vector{float32(float64(axis[0]) / magnitude), float32(float64(axis[1]) / magnitude), float32(float64(axis[2]) / magnitude)}
}

func rotate(a, b Vector, cos, sin, size float32) Vector {
	return Vector{(a[0]*cos + b[0]*sin) / size, (a[1]*cos + b[1]*sin) / size, (a[2]*cos + b[2]*sin) / size}
}
