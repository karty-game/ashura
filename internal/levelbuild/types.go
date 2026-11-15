//go:build !js

// Package levelbuild partitions placed static geometry during asset preparation.
// It contains no runtime culling, portal traversal or resource-loading behavior.
package levelbuild

import (
	"errors"

	"github.com/karty-game/karty-webgpu/internal/modeldata"
)

var ErrGeometry = errors.New("invalid or oversized level geometry")

const (
	MaxChunks    = 4096
	MaxTriangles = 1 << 20
)

// Placement describes one already prepared model in a resolved logical space.
// Transform is column-major world space; zero selects identity. PartGroups can
// distinguish external material overrides absent from the model's material ABI.
type Placement struct {
	ID, Space, Group string
	Model            modeldata.Document
	Transform        [16]float32
	Dynamic          bool
	PartGroups       map[string]string
}

// Options bounds partition dimensions, placement and output allocation.
// Zero Size selects 32-unit cells; zero limits select package maxima.
type Options struct {
	Size, Origin            [3]float32
	MaxChunks, MaxTriangles uint32
}

// SourceRange maps a merged index range back to its logical placement and
// original primitive. FirstIndex and IndexCount count uint32 indices, not bytes.
type SourceRange struct {
	ID, Name, Group        string
	Part                   int
	FirstIndex, IndexCount uint32
}

// Chunk owns a cell's clipped, merged geometry in the same world coordinates
// as its visibility scene. Origin supplies a stable later localization point.
type Chunk struct {
	ID, Space        string
	Cell             [3]int32
	Origin, Min, Max [3]float32
	Model            modeldata.Document
	Sources          []SourceRange
}

// SpaceScene contains all static occluders for one independently owned space.
// Coincident geometry in different spaces never acquires shared ownership.
type SpaceScene struct {
	ID    string
	Model modeldata.Document
}

// Result owns generated geometry. Prepared texture byte slices remain immutable
// references to the input model payloads and are deduplicated in each document.
type Result struct {
	Chunks []Chunk
	Spaces []SpaceScene
}

type vector [3]float64

type vertex struct {
	position, normal vector
	uv               [2]float64
}

type triangle [3]vertex

type cellKey struct {
	space string
	cell  [3]int32
}
