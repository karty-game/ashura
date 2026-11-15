//go:build !js

//nolint:mnd,gosec // Bounded wasm32 UV-mesh fields and source vertex coordinates.
package xatlas

import (
	"context"
	"encoding/binary"
	"math"
)

func (guest atlasGuest) invalidCharts(atlas rawAtlas, sources [][]byte) ([]bool, bool) {
	invalid := make([]bool, atlas.charts)
	needed := false

	for part, mesh := range atlas.meshes {
		if err := guest.ctx.Err(); err != nil {
			panic(err)
		}

		for _, faces := range mesh.charts {
			checkRepairContext(guest.ctx)

			if len(faces) == 0 {
				panic(errFormat)
			}

			if chartOverlaps(guest.ctx, mesh, faces) || chartCollapsed(guest.ctx, mesh, faces, sources[part]) {
				chart := mesh.vertices[mesh.indices[faces[0]*3]].chart
				if chart < 0 || uint32(chart) >= atlas.charts {
					panic(errFormat)
				}

				invalid[chart] = true
				needed = true
			}
		}
	}

	return invalid, needed
}

func (guest atlasGuest) repairMesh(
	mesh rawMesh,
	source []byte,
	invalid []bool,
	material uint32,
	density float32,
) ([][2]float32, []uint32, []uint32, []uint32, uint32) {
	uvs := make([][2]float32, 0, len(mesh.vertices))
	refs := make([]uint32, 0, len(mesh.vertices))
	indices := append([]uint32(nil), mesh.indices...)
	materials := make([]uint32, len(indices)/3)

	for index, vertex := range mesh.vertices {
		if index%256 == 0 {
			checkRepairContext(guest.ctx)
		}

		uvs = append(uvs, [2]float32{vertex.uv[0] * 1024, vertex.uv[1] * 1024})
		refs = append(refs, vertex.original)
	}

	for face := range materials {
		if face%256 == 0 {
			if err := guest.ctx.Err(); err != nil {
				panic(err)
			}
		}

		chart := mesh.vertices[mesh.indices[face*3]].chart

		split := chart >= 0 && invalid[chart]
		if chart >= 0 && !split {
			materials[face] = uint32(chart)
		} else {
			materials[face] = material
			material++
		}

		if !split {
			continue
		}

		coordinates := projectFace(mesh, uint32(face), source, density)

		for corner := range 3 {
			if len(refs) >= 1<<20 {
				panic(errFormat)
			}

			indices[face*3+corner] = uint32(len(refs))
			refs = append(refs, mesh.vertices[mesh.indices[face*3+corner]].original)
			uvs = append(uvs, coordinates[corner])
		}
	}

	return uvs, refs, indices, materials, material
}

func projectFace(mesh rawMesh, face uint32, source []byte, density float32) [3][2]float32 {
	ab, ac, area := worldEdges(mesh, face, source)

	length, projection := 0.0, 0.0
	for axis := range 3 {
		length += ab[axis] * ab[axis]
		projection += ab[axis] * ac[axis]
	}

	length = math.Sqrt(length)
	scale := float64(density) * 1024

	coordinates := [3][2]float32{{0, 0}, {float32(length * scale), 0}, {0, 0}}
	if length > 0 {
		coordinates[2] = [2]float32{float32(projection / length * scale), float32(area / length * scale)}
	}

	return coordinates
}

func (guest atlasGuest) addRepairMesh(next uint32, uvs [][2]float32, indices, materials []uint32) {
	var uvBytes, indexBytes, materialBytes []byte

	for index, coordinates := range uvs {
		if index%256 == 0 {
			checkRepairContext(guest.ctx)
		}

		uvBytes = binary.LittleEndian.AppendUint32(uvBytes, math.Float32bits(coordinates[0]))
		uvBytes = binary.LittleEndian.AppendUint32(uvBytes, math.Float32bits(coordinates[1]))
	}

	for offset, index := range indices {
		if offset%256 == 0 {
			checkRepairContext(guest.ctx)
		}

		indexBytes = binary.LittleEndian.AppendUint32(indexBytes, index)
	}

	for index, value := range materials {
		if index%256 == 0 {
			checkRepairContext(guest.ctx)
		}

		materialBytes = binary.LittleEndian.AppendUint32(materialBytes, value)
	}

	vertexPointer, indexPointer, materialPointer, decl := guest.copy(
		uvBytes,
	), guest.copy(
		indexBytes,
	), guest.copy(
		materialBytes,
	), guest.alloc(
		32,
	)
	guest.call("xatlasUvMeshDeclInit", uint64(decl))

	for _, field := range [][2]uint32{{0, vertexPointer}, {4, indexPointer}, {8, materialPointer}, {12, uint32(len(uvs))}, {16, 8}, {20, uint32(len(indices))}, {28, 1}} {
		guest.put(decl+field[0], field[1])
	}

	if guest.call("xatlasAddUvMesh", uint64(next), uint64(decl)) != 0 {
		panic(errFormat)
	}

	for _, pointer := range []uint32{vertexPointer, indexPointer, materialPointer, decl} {
		guest.call("free", uint64(pointer))
	}
}

func remapRepairedMesh(ctx context.Context, mesh *rawMesh, refs []uint32) {
	for vertexIndex := range mesh.vertices {
		if vertexIndex%256 == 0 {
			checkRepairContext(ctx)
		}

		vertex := &mesh.vertices[vertexIndex]
		if int(vertex.original) >= len(refs) {
			panic(errFormat)
		}

		vertex.original = refs[vertex.original]
	}

	for offset := 0; offset < len(mesh.indices); offset += 3 {
		if offset%256 == 0 {
			checkRepairContext(ctx)
		}

		ignored := false
		for corner := range 3 {
			ignored = ignored || mesh.vertices[mesh.indices[offset+corner]].chart < 0
		}

		if !ignored {
			continue
		}

		for corner := range 3 {
			vertex := mesh.vertices[mesh.indices[offset+corner]]
			vertex.chart, vertex.page, vertex.uv = -1, -1, [2]float32{}

			if len(mesh.vertices) >= 1<<20 {
				panic(errFormat)
			}

			mesh.indices[offset+corner] = uint32(len(mesh.vertices))
			mesh.vertices = append(mesh.vertices, vertex)
		}
	}

	for vertexIndex := range mesh.vertices {
		if vertexIndex%256 == 0 {
			checkRepairContext(ctx)
		}

		if mesh.vertices[vertexIndex].chart < 0 {
			mesh.vertices[vertexIndex].uv = [2]float32{}
		}
	}
}
