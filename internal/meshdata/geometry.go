// Package meshdata admits immutable GPU-ready indexed geometry.
package meshdata

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
)

const (
	// VertexStride is position.xyz, normal.xyz and uv.xy as little-endian float32.
	VertexStride = 32
	// IndexStride is one little-endian uint32 triangle index.
	IndexStride = 4
	// MaxVertices and MaxIndices bound one admitted geometry.
	MaxVertices     = 1 << 20
	MaxIndices      = 3 << 20
	triangleIndices = 3
)

var (
	errBounds = errors.New("prepared mesh has invalid vertex/index bounds")
	errFinite = errors.New("prepared mesh contains non-finite vertex data")
	errIndex  = errors.New("prepared mesh index is outside the vertex array")
)

// Geometry owns validated, immutable bytes in the renderer's exact GPU layout.
type Geometry struct {
	vertices, indices []byte
	lightmapUV        []byte
	bounds            Bounds
}

// Bounds is a conservative local-space box prepared with the model offline.
type Bounds struct {
	Minimum, Maximum [3]float32
	Valid            bool
}

// New validates and copies caller-owned GPU-ready geometry.
func New(vertices, indices []byte) (*Geometry, error) {
	return NewWithBounds(vertices, indices, Bounds{})
}

// NewWithBounds copies and admits a caller-owned prepared model part.
func NewWithBounds(vertices, indices []byte, bounds Bounds) (*Geometry, error) {
	geometry, err := FromOwnedWithBounds(vertices, indices, bounds)
	if err != nil {
		return nil, err
	}

	geometry.vertices = append([]byte(nil), vertices...)
	geometry.indices = append([]byte(nil), indices...)

	return geometry, nil
}

// FromOwned admits slices whose backing storage is already privately owned by
// an internal asset loader. The caller must never change the admitted bytes.
func FromOwned(vertices, indices []byte) (*Geometry, error) {
	return FromOwnedWithBounds(vertices, indices, Bounds{})
}

// FromOwnedWithBounds verifies offline metadata in the existing vertex admission
// walk. It does not derive a runtime box or make another pass over vertices.
func FromOwnedWithBounds(vertices, indices []byte, bounds Bounds) (*Geometry, error) {
	if len(vertices) == 0 || len(vertices)%VertexStride != 0 || len(vertices)/VertexStride > MaxVertices ||
		len(indices) == 0 || len(indices)%IndexStride != 0 || len(indices)/IndexStride > MaxIndices ||
		len(indices)/IndexStride%triangleIndices != 0 {
		return nil, errBounds
	}

	if bounds.Valid && !bounds.finite() {
		return nil, errBounds
	}

	for start := 0; start < len(vertices); start += VertexStride {
		for component := range VertexStride / IndexStride {
			offset := start + component*IndexStride

			value := math.Float32frombits(binary.LittleEndian.Uint32(vertices[offset:]))
			if !finite(value) {
				return nil, fmt.Errorf("%w at byte %d", errFinite, offset)
			}

			if bounds.Valid && component < len(bounds.Minimum) && (value < bounds.Minimum[component] || value > bounds.Maximum[component]) {
				return nil, errBounds
			}
		}
	}

	for offset := 0; offset < len(indices); offset += IndexStride {
		index := binary.LittleEndian.Uint32(indices[offset:])
		if uint64(index) >= uint64(len(vertices)/VertexStride) {
			return nil, fmt.Errorf("%w: %d", errIndex, index)
		}
	}

	return &Geometry{vertices: vertices, indices: indices, bounds: bounds}, nil
}

// WithLightmapUV attaches a prepared, non-repeating secondary UV stream. The
// returned geometry shares immutable position/index storage and owns its UV copy.
// This is admission of build output, never an unwrap or geometry conversion.
func (geometry *Geometry) WithLightmapUV(data []byte) (*Geometry, error) {
	if geometry == nil || len(data) != len(geometry.vertices)/VertexStride*8 {
		return nil, errBounds
	}

	for offset := 0; offset < len(data); offset += 4 {
		value := math.Float32frombits(binary.LittleEndian.Uint32(data[offset:]))
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) || value < 0 || value > 1 {
			return nil, fmt.Errorf("lightmap UV at byte %d: %w", offset, errFinite)
		}
	}

	return &Geometry{
		vertices:   geometry.vertices,
		indices:    geometry.indices,
		lightmapUV: append([]byte(nil), data...),
		bounds:     geometry.bounds,
	}, nil
}

// LightmapUV returns the immutable prepared float32x2 secondary UV stream.
func (geometry *Geometry) LightmapUV() []byte { return geometry.lightmapUV }

// Bounds returns the admitted box, if the asset supplied one.
func (geometry *Geometry) Bounds() Bounds { return geometry.bounds }

func finite(value float32) bool { return !math.IsInf(float64(value), 0) && !math.IsNaN(float64(value)) }

func (bounds Bounds) finite() bool {
	for axis, minimum := range bounds.Minimum {
		maximum := bounds.Maximum[axis]
		if !finite(minimum) || !finite(maximum) || minimum > maximum {
			return false
		}
	}

	return true
}

// Vertices returns an immutable internal upload view, never a mutable buffer.
func (geometry *Geometry) Vertices() []byte { return geometry.vertices }

// Indices returns an immutable internal upload view, never a mutable buffer.
func (geometry *Geometry) Indices() []byte { return geometry.indices }

// Release invalidates the owned CPU upload views after the asset owner retires
// its live entities, or during App.Close. Submitted GPU work owns its GPU copies.
func (geometry *Geometry) Release() {
	if geometry != nil {
		geometry.vertices = nil
		geometry.indices = nil
		geometry.lightmapUV = nil
	}
}
