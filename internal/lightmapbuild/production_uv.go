//go:build !js

package lightmapbuild

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"

	"github.com/karty-game/ashura/internal/lightmapper"
	"github.com/karty-game/ashura/internal/modeldata"
	"github.com/karty-game/ashura/internal/xatlas"
)

// UVPage retains exact footprint preparation without a visibility hierarchy,
// legacy .klightmap or center-sampled transport records. ChartOwners
// marks actual clipped coverage; GutterOwners maps output pixels to covered
// pixels within the same chart, or MaxUint32 for unused output.
type UVPage struct {
	Model                 []byte
	UVs                   [][]byte
	Width, Height, Charts uint32
	ChartOwners           []int32
	GutterOwners          []uint32
	TriangleCharts        [][]int32
	// Receivers retain the exact footprint work used to validate chart coverage.
	// Their pieces use original model triangle indices, including skipped faces.
	Receivers []lightmapper.Receiver
}

// UVPreparer owns the embedded xatlas library for a build.
// Geometry acceleration is a separate cache action and is never built here.
type UVPreparer struct{ atlas *xatlas.Session }

func NewUVPreparer(ctx context.Context) (*UVPreparer, error) {
	if ctx == nil {
		return nil, errGeometry
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	atlas, err := xatlas.OpenSession(ctx)
	if err != nil {
		return nil, err
	}

	return &UVPreparer{atlas: atlas}, nil
}

func (p *UVPreparer) Close() error {
	if p == nil || p.atlas == nil {
		return nil
	}

	err := p.atlas.Close()
	p.atlas = nil

	return err
}

func (p *UVPreparer) Prepare(ctx context.Context, model modeldata.Document, options PageOptions) ([]UVPage, error) {
	var output []UVPage

	err := p.PrepareEach(ctx, model, options, func(page UVPage) error {
		output = append(output, page)

		return nil
	})
	if err != nil {
		return nil, err
	}

	return output, nil
}

// PrepareEach consumes one validated page before constructing the next page's
// footprints. Production stores that page immediately, avoiding retention of a
// complete group's receiver allocations just to remove duplicate clipping.
func (p *UVPreparer) PrepareEach(ctx context.Context, model modeldata.Document, options PageOptions, consume func(UVPage) error) error {
	if ctx == nil || p == nil || p.atlas == nil || consume == nil {
		return errGeometry
	}

	if err := ctx.Err(); err != nil {
		return err
	}
	// This API charts RGB and RNM equally; legacy transport flags have no role.
	options.PrecomputeTransport = false
	options.PreserveGeometricNormals = false

	options, err := normalizePageOptions(options)
	if err != nil {
		return err
	}

	if _, err = modeldata.Encode(model); err != nil {
		return err
	}

	pages, err := p.atlas.GeneratePages(
		ctx,
		model.Parts,
		xatlas.PageOptions{
			TexelsPerUnit: options.TexelsPerUnit,
			Resolution:    options.MaxPageSize,
			Padding:       options.Padding,
			MaxPages:      options.MaxPages,
		},
	)
	if err != nil {
		return err
	}
	// Reuse only the atlas-capacity helper, with no legacy visibility state.
	helper := Preparer{atlas: p.atlas}

	pages, err = helper.smallestSinglePage(ctx, model.Parts, pages, options)
	if err != nil {
		return err
	}

	for _, page := range pages {
		if err := ctx.Err(); err != nil {
			return err
		}

		prepared, err := productionUVPage(ctx, model, page)
		if err != nil {
			return err
		}

		if err = consume(prepared); err != nil {
			return err
		}
	}

	return nil
}

func productionUVPage(ctx context.Context, source modeldata.Document, page xatlas.Page) (UVPage, error) {
	model := productionPageModel(source, page.PartIndices)
	uvs := remapModel(&model, page.Atlas)

	encoded, err := modeldata.Encode(model)
	if err != nil {
		return UVPage{}, err
	}

	result := UVPage{
		Model:          encoded,
		UVs:            uvs,
		Width:          page.Width,
		Height:         page.Height,
		Charts:         page.Charts,
		TriangleCharts: make([][]int32, len(model.Parts)),
	}

	var (
		triangles    []lightmapper.Triangle
		triangleUV   [][3]lightmapper.Vec2
		charts       []int32
		modelIndices []uint32
		modelIndex   uint32
	)

	for partIndex, part := range model.Parts {
		mesh := page.Meshes[partIndex]

		result.TriangleCharts[partIndex] = make([]int32, len(mesh.Indices)/3)
		for offset := 0; offset < len(mesh.Indices); offset += 3 {
			triangle, coordinates, chart, triangleErr := productionUVTriangle(part, mesh, uvs[partIndex], offset, uint64(modelIndex)+1)
			originalIndex := modelIndex
			modelIndex++

			if triangleErr != nil {
				return UVPage{}, triangleErr
			}

			triangle.Material = uint32(partIndex)

			if chart < 0 &&
				lightmapper.Length(
					lightmapper.Cross(
						lightmapper.Sub(triangle.Positions[1], triangle.Positions[0]),
						lightmapper.Sub(triangle.Positions[2], triangle.Positions[0]),
					),
				) == 0 {
				result.TriangleCharts[partIndex][offset/3] = -1

				continue
			}

			if chart < 0 || uint32(chart) >= page.Charts {
				return UVPage{}, errGeometry
			}

			result.TriangleCharts[partIndex][offset/3] = chart

			triangles = append(triangles, triangle)
			triangleUV = append(triangleUV, coordinates)
			charts = append(charts, chart)
			modelIndices = append(modelIndices, originalIndex)
		}
	}

	return productionUVCoverage(ctx, page, result, triangles, triangleUV, charts, modelIndices)
}

func productionPageModel(source modeldata.Document, indices []int) modeldata.Document {
	model := modeldata.Document{Source: source.Source, Textures: source.Textures, Parts: make([]modeldata.Part, len(indices))}
	for i, index := range indices {
		model.Parts[i] = source.Parts[index]
	}

	prunePageTextures(&model)

	return model
}

//nolint:mnd // The admitted model uses the established 32-byte position/normal/UV0 vertex layout.
func productionUVTriangle(
	part modeldata.Part,
	mesh xatlas.Mesh,
	uvs []byte,
	offset int,
	triangleID uint64,
) (lightmapper.Triangle, [3]lightmapper.Vec2, int32, error) {
	triangle := lightmapper.Triangle{ID: triangleID}

	var coordinates [3]lightmapper.Vec2

	chart := mesh.Vertices[mesh.Indices[offset]].Chart
	for corner := range 3 {
		index := mesh.Indices[offset+corner]
		if mesh.Vertices[index].Chart != chart {
			return lightmapper.Triangle{}, [3]lightmapper.Vec2{}, 0, fmt.Errorf("mixed triangle chart: %w", errGeometry)
		}

		base := int(index) * 32
		for axis := range 3 {
			triangle.Positions[corner][axis] = readFloat(part.Vertices[base+axis*4:])
			triangle.Normals[corner][axis] = readFloat(part.Vertices[base+12+axis*4:])
		}

		for axis := range 2 {
			triangle.UV[corner][axis] = readFloat(part.Vertices[base+24+axis*4:])
			coordinates[corner][axis] = readFloat(uvs[int(index)*8+axis*4:])
		}
	}

	return triangle, coordinates, chart, nil
}

func productionUVCoverage(
	ctx context.Context,
	page xatlas.Page,
	result UVPage,
	triangles []lightmapper.Triangle,
	triangleUV [][3]lightmapper.Vec2,
	charts []int32,
	modelIndices []uint32,
) (UVPage, error) {
	receivers, err := lightmapper.PrepareReceivers(triangles, triangleUV, page.Width, page.Height)
	if err != nil {
		return UVPage{}, fmt.Errorf(
			"production atlas (%dx%d, %d triangles, %d charts): %w",
			page.Width,
			page.Height,
			len(triangles),
			page.Charts,
			err,
		)
	}

	result.ChartOwners = make([]int32, int(page.Width)*int(page.Height))
	for i := range result.ChartOwners {
		result.ChartOwners[i] = -1
	}

	for index := range receivers {
		receiver := &receivers[index]
		owner := int32(-1)

		for _, piece := range receiver.Pieces {
			chart := charts[piece.Triangle]
			if owner >= 0 && owner != chart {
				return UVPage{}, fmt.Errorf("overlapping chart footprints: %w", errGeometry)
			}

			owner = chart
		}

		result.ChartOwners[receiver.Pixel] = owner
		for pieceIndex := range receiver.Pieces {
			receiver.Pieces[pieceIndex].Triangle = modelIndices[receiver.Pieces[pieceIndex].Triangle]
		}

		for patchIndex := range receiver.Patches {
			receiver.Patches[patchIndex].Triangle = math.MaxUint32
		}
	}

	result.Receivers = receivers

	padding, err := paddingLookup(ctx, page.Atlas, result.ChartOwners)
	if err != nil {
		return UVPage{}, err
	}

	result.GutterOwners = make([]uint32, len(padding)/4)
	for i := range result.GutterOwners {
		result.GutterOwners[i] = binary.LittleEndian.Uint32(padding[i*4:])
		if result.GutterOwners[i] != math.MaxUint32 && result.ChartOwners[result.GutterOwners[i]] < 0 {
			return UVPage{}, errGeometry
		}
	}

	return result, nil
}
