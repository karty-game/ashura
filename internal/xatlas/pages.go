//go:build !js

//nolint:gosec // Counts and wire words are bounded before allocation/conversion by the private atlas protocol.
package xatlas

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"math"

	"github.com/karty-game/karty-webgpu/internal/meshdata"
	"github.com/karty-game/karty-webgpu/internal/modeldata"
)

// Page maps one atlas page's nonempty primitives to the supplied source parts.
// Chart identifiers are compact and local to this page.
type Page struct {
	Atlas

	PartIndices   []int
	TexelsPerUnit float32
}

// PageOptions selects physical density and fixed atlas dimensions. MaxPages is
// bounded at64; the complete atlas response still cannot exceed128MiB.
type PageOptions struct {
	TexelsPerUnit       float32
	Resolution, Padding uint32
	MaxPages            uint32
}

// GeneratePages unwraps a receiver group at an explicit physical density. It
// never reduces density to fit an oversized chart; callers may split geometry.
func (session *Session) GeneratePages(ctx context.Context, parts []modeldata.Part, options PageOptions) ([]Page, error) {
	if ctx == nil || session == nil || session.wasm == nil || !validPageRequest(parts, options) {
		return nil, errFormat
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var input bytes.Buffer
	for _, word := range []uint32{0x3258414b, uint32(len(parts)), options.Resolution, options.Padding,
		math.Float32bits(options.TexelsPerUnit), options.MaxPages} {
		_ = binary.Write(&input, binary.LittleEndian, word)
	}

	var vertexCount, indexCount uint64

	for _, part := range parts {
		if _, err := meshdata.FromOwned(part.Vertices, part.Indices); err != nil {
			return nil, err
		}

		vertexCount += uint64(len(part.Vertices) / meshdata.VertexStride)

		indexCount += uint64(len(part.Indices) / meshdata.IndexStride)
		if vertexCount > meshdata.MaxVertices || indexCount > meshdata.MaxIndices ||
			input.Len()+len(part.Vertices)+len(part.Indices)+8 > modeldata.MaxBytes {
			return nil, errFormat
		}

		_ = binary.Write(&input, binary.LittleEndian, uint32(len(part.Vertices)/meshdata.VertexStride))
		_ = binary.Write(&input, binary.LittleEndian, uint32(len(part.Indices)/meshdata.IndexStride))
		input.Write(part.Vertices)
		input.Write(part.Indices)
	}

	output, err := session.wasm.execute(ctx, input.Bytes())
	if err != nil {
		return nil, err
	}

	return decodePages(output, parts, options)
}

type pageVertex struct {
	Vertex

	page int32
}

type pageMesh struct {
	vertices []pageVertex
	indices  []uint32
}

func decodePages(data []byte, parts []modeldata.Part, options PageOptions) ([]Page, error) {
	input := reader{data: data}
	if input.word() != 0x32584f4b || input.word() != uint32(len(parts)) {
		return nil, errFormat
	}

	width, height, charts, count := input.word(), input.word(), input.word(), input.word()

	density := math.Float32frombits(input.word())
	if !validPageHeader(width, height, charts, count, density, options) {
		return nil, errFormat
	}

	meshes := make([]pageMesh, len(parts))

	var vertices uint64

	for index, part := range parts {
		mesh, err := decodePageMesh(&input, part, width, height, charts, count)
		if err != nil {
			return nil, fmt.Errorf("mesh %d (%s): %w", index, part.Name, err)
		}

		vertices += uint64(len(mesh.vertices))
		if vertices > meshdata.MaxVertices {
			return nil, errFormat
		}

		meshes[index] = mesh
	}

	texels := uint64(width) * uint64(height)
	if texels*uint64(count)*4 != uint64(len(data)-input.offset) {
		return nil, errFormat
	}

	pages := make([]Page, count)
	for pageIndex := range pages {
		page, err := splitPage(pageIndex, meshes, charts)
		if err != nil {
			return nil, err
		}

		page.Width, page.Height, page.TexelsPerUnit = width, height, density
		if err := readPageImage(&input, &page, texels); err != nil {
			return nil, fmt.Errorf("page %d occupancy: %w", pageIndex, err)
		}

		if err := validatePageDensity(page, parts, options.TexelsPerUnit); err != nil {
			return nil, err
		}

		pages[pageIndex] = page
	}

	if input.err != nil {
		return nil, input.err
	}

	return pages, nil
}

func decodePageMesh(input *reader, part modeldata.Part, width, height, charts, pages uint32) (pageMesh, error) {
	count, indices := input.word(), input.word()
	if count == 0 || count > meshdata.MaxVertices || indices != uint32(len(part.Indices)/4) ||
		uint64(count)*20+uint64(indices)*4 > uint64(len(input.data)-input.offset) {
		return pageMesh{}, fmt.Errorf(
			"mesh header vertices %d indices %d (want %d), remaining %d: %w",
			count,
			indices,
			len(part.Indices)/4,
			len(input.data)-input.offset,
			errFormat,
		)
	}

	mesh := pageMesh{vertices: make([]pageVertex, count), indices: make([]uint32, indices)}
	for index := range mesh.vertices {
		v := &mesh.vertices[index]
		v.Original, v.Chart, v.page = input.word(), int32(input.word()), int32(input.word())

		v.UV = [2]float32{math.Float32frombits(input.word()), math.Float32frombits(input.word())}
		if !validPageVertex(*v, part, width, height, charts, pages) {
			return pageMesh{}, fmt.Errorf("vertex %d (%+v): %w", index, *v, errFormat)
		}
	}

	for index := range mesh.indices {
		mesh.indices[index] = input.word()
		if mesh.indices[index] >= count {
			return pageMesh{}, errFormat
		}

		original := binary.LittleEndian.Uint32(part.Indices[index*4:])
		if mesh.vertices[mesh.indices[index]].Original != original {
			return pageMesh{}, fmt.Errorf(
				"index %d source changed from %d to %d: %w",
				index,
				original,
				mesh.vertices[mesh.indices[index]].Original,
				errFormat,
			)
		}
	}

	if err := validateIgnoredTriangles(mesh, part); err != nil {
		return pageMesh{}, fmt.Errorf("ignored triangles: %w", err)
	}

	return mesh, input.err
}

func validPageRequest(parts []modeldata.Part, options PageOptions) bool {
	return len(parts) > 0 && len(parts) <= modeldata.MaxParts && finite(options.TexelsPerUnit) && options.TexelsPerUnit > 0 &&
		options.TexelsPerUnit <= 4096 &&
		options.Resolution >= 16 &&
		options.Resolution <= 2048 &&
		options.Padding >= 1 &&
		options.Padding <= 16 &&
		options.Padding*2+2 < options.Resolution &&
		options.MaxPages > 0 &&
		options.MaxPages <= 64
}
func validPageHeader(width, height, charts, count uint32, density float32, options PageOptions) bool {
	return width == options.Resolution && height == options.Resolution && charts > 0 && charts <= meshdata.MaxIndices/3 && count > 0 &&
		count <= options.MaxPages &&
		finite(density) &&
		density == options.TexelsPerUnit
}

func validPageVertex(value pageVertex, part modeldata.Part, width, height, charts, pages uint32) bool {
	return value.Original < uint32(len(part.Vertices)/32) && value.Chart >= -1 && int64(value.Chart) < int64(charts) && value.page >= -1 &&
		int64(value.page) < int64(pages) &&
		(value.Chart == -1) == (value.page == -1) &&
		finite(value.UV[0]) &&
		finite(value.UV[1]) &&
		value.UV[0] >= 0 &&
		value.UV[1] >= 0 &&
		value.UV[0] <= float32(width) &&
		value.UV[1] <= float32(height)
}

func readPageImage(input *reader, page *Page, texels uint64) error {
	page.Image = make([]uint32, int(texels))

	chartMap := pageChartMap(*page)
	for index := range page.Image {
		pixel := input.word()
		if pixel == 0 {
			continue
		}

		chart, exists := chartMap[pixel&0x1fffffff]
		if !exists {
			return errFormat
		}

		page.Image[index] = pixel&^uint32(0x1fffffff) | chart
	}

	compactPageCharts(page, chartMap)

	return input.err
}
