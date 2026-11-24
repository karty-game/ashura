// Package collision provides CPU-side static collision and scene queries in
// Ashura world coordinates. A World belongs to the application's update loop.
package collision

import (
	"math"
	"slices"

	"github.com/karty-game/ashura/pkg/engine"
)

// ID identifies one collider in a World. Zero never identifies a collider.
type ID uint64

// Layers is a bit mask. A collider participates when any query bit matches.
type Layers uint32

// Bounds is a world-space axis-aligned box.
type Bounds struct {
	Min, Max engine.Vector3
}

// Triangle is a double-sided world-space surface.
type Triangle struct {
	A, B, C engine.Vector3
}

// NodeV1 is one node in a generated static mesh. Skip is the index immediately
// after its subtree. A nonzero Count identifies a leaf's triangle range.
type NodeV1 struct {
	Bounds       Bounds
	Skip         uint32
	First, Count uint32
}

// Mesh is immutable world-space geometry produced by the offline collision build.
type Mesh struct {
	nodes     []NodeV1
	triangles []Triangle
}

// PreparedMeshV1 binds compiler-generated Go data. The generator owns the
// topology and bounds. The backing arrays must remain immutable.
func PreparedMeshV1(nodes []NodeV1, triangles []Triangle) *Mesh {
	return &Mesh{nodes: nodes, triangles: triangles}
}

// Capsule describes a vertical capsule centered on the query position.
// HalfSegment is half the distance between the spherical cap centers.
type Capsule struct {
	Radius, HalfSegment float32
}

// Hit is the first contact along a displacement. Fraction ranges from zero to
// one and Point is on the collider surface. Normal faces the approaching shape.
type Hit struct {
	ID       ID
	Point    engine.Vector3
	Normal   engine.Vector3
	Fraction float32
}

type collider struct {
	mesh   *Mesh
	box    Bounds
	id     ID
	layers Layers
}

// World owns the active mesh instances and trigger volumes. Its zero value
// is ready to use. IDs are assigned in insertion order and are never reused.
type World struct {
	colliders []collider
	next      ID
}

// AddMesh registers a prepared mesh and returns its ID. A nil or empty mesh
// or a zero layer mask returns zero.
func (world *World) AddMesh(mesh *Mesh, layers Layers) ID {
	if mesh == nil || len(mesh.nodes) == 0 || len(mesh.triangles) == 0 {
		return 0
	}

	return world.add(collider{mesh: mesh, layers: layers})
}

// AddTrigger registers an axis-aligned, nonblocking trigger. Invalid bounds
// or a zero layer mask return zero.
func (world *World) AddTrigger(box Bounds, layers Layers) ID {
	lo, hi := fromEngine(box.Min), fromEngine(box.Max)
	if !lo.finite() || !hi.finite() || lo[0] > hi[0] || lo[1] > hi[1] || lo[2] > hi[2] {
		return 0
	}

	return world.add(collider{box: box, layers: layers})
}

// Remove unregisters a mesh or trigger and reports whether it was present.
func (world *World) Remove(id ID) bool {
	index, found := slices.BinarySearchFunc(world.colliders, id, func(entry collider, target ID) int {
		switch {
		case entry.id < target:
			return -1
		case entry.id > target:
			return 1
		default:
			return 0
		}
	})
	if found {
		world.colliders = slices.Delete(world.colliders, index, index+1)
	}

	return found
}

// AppendTriggers appends the IDs of matching triggers that touch or overlap
// the capsule, in insertion order. Pass a reusable slice to avoid allocations.
func (world *World) AppendTriggers(dst []ID, center engine.Vector3, shape Capsule, layers Layers) []ID {
	position := fromEngine(center)
	if !shape.valid() || !position.finite() || layers == 0 {
		return dst
	}

	radius, half := float64(shape.Radius), float64(shape.HalfSegment)
	for _, entry := range world.colliders {
		if entry.mesh == nil && entry.layers&layers != 0 && overlapsBox(position, radius, half, entry.box) {
			dst = append(dst, entry.id)
		}
	}

	return dst
}

func (world *World) add(entry collider) ID {
	if entry.layers == 0 || world.next == math.MaxUint64 {
		return 0
	}

	world.next++
	entry.id = world.next
	world.colliders = append(world.colliders, entry)

	return entry.id
}

func overlapsBox(center vector, radius, half float64, box Bounds) bool {
	lo, hi := fromEngine(box.Min), fromEngine(box.Max)
	dx := max(lo[0]-center[0], center[0]-hi[0], 0)
	dy := max(lo[1]-center[1]-half, center[1]-half-hi[1], 0)
	dz := max(lo[2]-center[2], center[2]-hi[2], 0)

	return dx*dx+dy*dy+dz*dz <= radius*radius
}

func (shape Capsule) valid() bool {
	radius, half := float64(shape.Radius), float64(shape.HalfSegment)

	return radius > 0 && half >= 0 && finite(radius) && finite(half)
}
