package collision

import (
	"math"

	"github.com/karty-game/ashura/pkg/engine"
)

const (
	// Capsule casts converge to within this many world units of contact.
	contactTolerance = 1e-6
	castIterations   = 32
	maxLeafTriangles = 8
)

type sweep struct {
	start, delta vector
	radius, half float64
	speed        float64
	tolerance    float64
}

type contact struct {
	point, normal vector
	fraction      float64
}

// Raycast returns the first mesh surface along origin + fraction*displacement.
func (world *World) Raycast(origin, displacement engine.Vector3, layers Layers) (Hit, bool) {
	return world.hit(sweep{start: fromEngine(origin), delta: fromEngine(displacement)}, layers)
}

// CapsuleCast sweeps a vertical capsule along displacement. A contact at the
// start blocks motion into a surface and permits motion parallel to or away
// from it. Casts do not resolve an initially embedded capsule.
func (world *World) CapsuleCast(center engine.Vector3, shape Capsule, displacement engine.Vector3, layers Layers) (Hit, bool) {
	if !shape.valid() {
		return Hit{}, false
	}

	return world.hit(capsuleSweep(fromEngine(center), fromEngine(displacement), shape), layers)
}

func capsuleSweep(start, delta vector, shape Capsule) sweep {
	radius := float64(shape.Radius)

	const relativeTolerance = 1e-5

	tolerance := min(contactTolerance, radius*relativeTolerance)

	return sweep{start: start, delta: delta, radius: radius, half: float64(shape.HalfSegment), tolerance: tolerance}
}

func (world *World) hit(query sweep, layers Layers) (Hit, bool) {
	id, best := world.cast(query, layers)
	if id == 0 {
		return Hit{}, false
	}

	return Hit{ID: id, Point: best.point.engine(), Normal: best.normal.engine(), Fraction: float32(best.fraction)}, true
}

func (world *World) cast(query sweep, layers Layers) (ID, contact) {
	if !query.start.finite() || !query.delta.finite() || query.delta == (vector{}) || layers == 0 {
		return 0, contact{}
	}

	query.speed = math.Sqrt(query.delta.dot(query.delta))
	best := contact{fraction: 1}

	var bestID ID

	for _, entry := range world.colliders {
		if entry.mesh == nil || entry.layers&layers == 0 {
			continue
		}

		if hit, ok := entry.mesh.cast(query, best.fraction); ok && (bestID == 0 || hit.fraction < best.fraction) {
			bestID, best = entry.id, hit
		}
	}

	return bestID, best
}

func (mesh *Mesh) cast(query sweep, limit float64) (contact, bool) {
	best := contact{fraction: limit}
	found := false

	for index := 0; index < len(mesh.nodes); {
		node := mesh.nodes[index]

		skip := int(node.Skip)
		if skip <= index || skip > len(mesh.nodes) {
			break
		}

		if !query.intersects(node.Bounds, best.fraction) {
			index = skip

			continue
		}

		if node.Count != 0 {
			best, found = mesh.castLeaf(query, node, best, found)
		}

		index++
	}

	return best, found
}

func (mesh *Mesh) castLeaf(query sweep, node NodeV1, best contact, found bool) (contact, bool) {
	end := uint64(node.First) + uint64(node.Count)
	if end > uint64(len(mesh.triangles)) || node.Count > maxLeafTriangles {
		return best, found
	}

	for _, triangle := range mesh.triangles[node.First:end] {
		hit, ok := query.triangle(triangle, best.fraction)
		if ok && (!found || hit.fraction < best.fraction) {
			best, found = hit, true
		}
	}

	return best, found
}

func (query sweep) intersects(bounds Bounds, limit float64) bool {
	minimum, maximum := fromEngine(bounds.Min), fromEngine(bounds.Max)

	extent := vector{query.radius, query.radius + query.half, query.radius}
	if query.radius != 0 {
		extent = extent.add(vector{query.tolerance, query.tolerance, query.tolerance})
	}

	near, far := 0.0, limit

	for axis, delta := range query.delta {
		lower := minimum[axis] - extent[axis] - query.start[axis]

		upper := maximum[axis] + extent[axis] - query.start[axis]
		if delta == 0 {
			if lower > 0 || upper < 0 {
				return false
			}

			continue
		}

		entry, exit := lower/delta, upper/delta

		near, far = max(near, min(entry, exit)), min(far, max(entry, exit))
		if near > far {
			return false
		}
	}

	return true
}

func (query sweep) triangle(triangle Triangle, limit float64) (contact, bool) {
	first, second, third := fromEngine(triangle.A), fromEngine(triangle.B), fromEngine(triangle.C)
	normal := second.sub(first).cross(third.sub(first))

	length := math.Sqrt(normal.dot(normal))
	if length == 0 || !finite(length) {
		return contact{}, false
	}

	face := surface{a: first, b: second, c: third, normal: normal.mul(1 / length)}
	if query.radius > 0 {
		return query.capsuleTriangle(face, limit)
	}

	return face.ray(query.start, query.delta, limit)
}
