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

	return &Geometry{vertices: geometry.vertices, indices: geometry.indices, lightmapUV: append([]byte(nil), data...)}, nil
}

// LightmapUV returns the immutable prepared float32x2 secondary UV stream.
func (geometry *Geometry) LightmapUV() []byte { return geometry.lightmapUV }

// New validates and copies caller-owned GPU-ready geometry.
func New(vertices, indices []byte) (*Geometry, error) {
	geometry, err := FromOwned(vertices, indices)
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
	if len(vertices) == 0 || len(vertices)%VertexStride != 0 || len(vertices)/VertexStride > MaxVertices ||
		len(indices) == 0 || len(indices)%IndexStride != 0 || len(indices)/IndexStride > MaxIndices ||
		len(indices)/IndexStride%triangleIndices != 0 {
		return nil, errBounds
	}

	for offset := 0; offset < len(vertices); offset += IndexStride {
		value := math.Float32frombits(binary.LittleEndian.Uint32(vertices[offset:]))
		if math.IsInf(float64(value), 0) || math.IsNaN(float64(value)) {
			return nil, fmt.Errorf("%w at byte %d", errFinite, offset)
		}
	}

	for offset := 0; offset < len(indices); offset += IndexStride {
		index := binary.LittleEndian.Uint32(indices[offset:])
		if uint64(index) >= uint64(len(vertices)/VertexStride) {
			return nil, fmt.Errorf("%w: %d", errIndex, index)
		}
	}

	return &Geometry{vertices: vertices, indices: indices}, nil
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
