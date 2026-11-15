package engine

import "github.com/karty-game/karty-webgpu/internal/render"

// Mesh slices are immutable. Their headers identify geometry without scanning
// static vertices; replacing any slice selects a new prepared record.
type meshGeometryKey struct {
	positions, normals *Vector3
	uvs                *Vector2
	indices            *uint32
	positionCount      int
	normalCount        int
	uvCount            int
	indexCount         int
}

type renderGeometry struct {
	vertices []render.Vertex
	indices  []uint32
	seen     bool
}

func sliceStart[T any](values []T) *T {
	if len(values) == 0 {
		return nil
	}

	return &values[0]
}

func (world *World) meshGeometry(positions, normals []Vector3, uvs []Vector2, indices []uint32) *renderGeometry {
	key := meshGeometryKey{
		positions: sliceStart(positions), normals: sliceStart(normals), uvs: sliceStart(uvs), indices: sliceStart(indices),
		positionCount: len(positions), normalCount: len(normals), uvCount: len(uvs), indexCount: len(indices),
	}
	if cached, ok := world.geometry[key]; ok {
		cached.seen = true

		return cached
	}

	geometry := &renderGeometry{vertices: make([]render.Vertex, len(positions)), indices: indices, seen: true}
	for index, position := range positions {
		geometry.vertices[index].Position = vectorArray(position)
		if index < len(normals) {
			geometry.vertices[index].Normal = vectorArray(normals[index])
		}

		if index < len(uvs) {
			geometry.vertices[index].UV = [2]float32{uvs[index].X, uvs[index].Y}
		}
	}

	if world.geometry == nil {
		world.geometry = make(map[meshGeometryKey]*renderGeometry)
	}

	world.geometry[key] = geometry

	return geometry
}

func (world *World) pruneGeometry() {
	for key, geometry := range world.geometry {
		if !geometry.seen {
			delete(world.geometry, key)
		}

		geometry.seen = false
	}
}
