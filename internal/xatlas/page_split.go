//go:build !js

//nolint:mnd,gosec,varnamelen // Helper counts are prebounded; local indices and numeric offsets describe its geometry wire format.
package xatlas

import (
	"encoding/binary"
	"fmt"
	"math"

	"github.com/karty-game/karty-webgpu/internal/modeldata"
)

func splitPage(pageIndex int, meshes []pageMesh, charts uint32) (Page, error) {
	page := Page{}

	for partIndex, source := range meshes {
		mesh := Mesh{}
		remap := make(map[uint32]uint32)

		for offset := 0; offset < len(source.indices); offset += 3 {
			a, b, c := source.indices[offset], source.indices[offset+1], source.indices[offset+2]

			first := source.vertices[a]
			if source.vertices[b].page != first.page || source.vertices[c].page != first.page ||
				source.vertices[b].Chart != first.Chart || source.vertices[c].Chart != first.Chart {
				return Page{}, errFormat
			}

			if first.page != int32(pageIndex) {
				continue
			}

			for _, oldIndex := range []uint32{a, b, c} {
				index, exists := remap[oldIndex]
				if !exists {
					index = uint32(len(mesh.Vertices))
					remap[oldIndex] = index
					mesh.Vertices = append(mesh.Vertices, source.vertices[oldIndex].Vertex)
				}

				mesh.Indices = append(mesh.Indices, index)
			}
		}

		if len(mesh.Indices) == 0 {
			continue
		}

		page.PartIndices = append(page.PartIndices, partIndex)
		page.Meshes = append(page.Meshes, mesh)
	}

	if len(page.Meshes) == 0 || charts == 0 {
		return Page{}, errFormat
	}

	return page, nil
}

func pageChartMap(page Page) map[uint32]uint32 {
	charts := make(map[uint32]uint32)

	for _, mesh := range page.Meshes {
		for _, vertex := range mesh.Vertices {
			chart := uint32(vertex.Chart)
			if _, exists := charts[chart]; !exists {
				charts[chart] = uint32(len(charts))
			}
		}
	}

	return charts
}

func compactPageCharts(page *Page, charts map[uint32]uint32) {
	page.Charts = uint32(len(charts))
	for part := range page.Meshes {
		for index := range page.Meshes[part].Vertices {
			vertex := &page.Meshes[part].Vertices[index]
			vertex.Chart = int32(charts[uint32(vertex.Chart)])
		}
	}
}

func validatePageDensity(page Page, parts []modeldata.Part, requested float32) error {
	areas := make([][2]float64, page.Charts)
	for partIndex, mesh := range page.Meshes {
		source := parts[page.PartIndices[partIndex]]

		for offset := 0; offset < len(mesh.Indices); offset += 3 {
			var positions [3][3]float64

			var uv [3][2]float32

			chart := int32(-1)

			for corner := range 3 {
				v := mesh.Vertices[mesh.Indices[offset+corner]]

				chart, uv[corner] = v.Chart, v.UV
				for axis := range 3 {
					positions[corner][axis] = float64(
						math.Float32frombits(binary.LittleEndian.Uint32(source.Vertices[int(v.Original)*32+axis*4:])),
					)
				}
			}

			var ab, ac, cross [3]float64
			for axis := range 3 {
				ab[axis] = positions[1][axis] - positions[0][axis]
				ac[axis] = positions[2][axis] - positions[0][axis]
			}

			for axis := range 3 {
				x, y := (axis+1)%3, (axis+2)%3
				cross[axis] = ab[x]*ac[y] - ab[y]*ac[x]
			}

			area := math.Sqrt(cross[0]*cross[0]+cross[1]*cross[1]+cross[2]*cross[2]) * .5
			uvArea := math.Abs(float64(uv[1][0]-uv[0][0])*float64(uv[2][1]-uv[0][1])-
				float64(uv[1][1]-uv[0][1])*float64(uv[2][0]-uv[0][0])) * .5
			areas[chart][0] += area
			areas[chart][1] += uvArea
		}
	}

	for _, area := range areas {
		if area[0] <= 1e-18 {
			continue
		}

		actual := math.Sqrt(area[1] / area[0])
		if actual < float64(requested)*.999 {
			return fmt.Errorf(
				"xatlas chart density %.4g is below requested %.4g texels/unit; subdivide receiver geometry or increase page size: %w",
				actual,
				requested,
				errFormat,
			)
		}
	}

	return nil
}

func validateIgnoredTriangles(mesh pageMesh, part modeldata.Part) error {
	for offset := 0; offset < len(mesh.indices); offset += 3 {
		first := mesh.vertices[mesh.indices[offset]]
		if first.page >= 0 {
			continue
		}

		var points [3][3]float64

		for corner := range 3 {
			vertex := mesh.vertices[mesh.indices[offset+corner]]
			if vertex.page != -1 {
				return fmt.Errorf(
					"triangle %d has mixed ignored corner %d (%+v), first %+v: %w",
					offset/3,
					corner,
					vertex,
					first,
					errFormat,
				)
			}

			for axis := range 3 {
				points[corner][axis] = float64(
					math.Float32frombits(binary.LittleEndian.Uint32(part.Vertices[int(vertex.Original)*32+axis*4:])),
				)
			}
		}

		var squared float64

		for axis := range 3 {
			a, b := (axis+1)%3, (axis+2)%3
			cross := (points[1][a]-points[0][a])*(points[2][b]-points[0][b]) - (points[1][b]-points[0][b])*(points[2][a]-points[0][a])
			squared += cross * cross
		}

		if squared > 0 {
			return fmt.Errorf("xatlas omitted a nondegenerate source triangle: %w", errFormat)
		}
	}

	return nil
}
