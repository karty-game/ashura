//go:build !js

//nolint:gosec // Geometry admission bounds all vertex/index casts and fixed GPU layout offsets.
package meshopt

import (
	"context"
	"encoding/binary"
	"math"
	"slices"

	"github.com/karty-game/ashura/internal/meshdata"
	"github.com/karty-game/ashura/internal/modeldata"
)

// Cleanup removes unused/exact duplicate vertices and exact zero-area faces.
// Parts remain independent; all surviving attributes and oriented corners are exact.
// Entirely degenerate parts are dropped; an empty model is rejected.
func Cleanup(ctx context.Context, source modeldata.Document) (modeldata.Document, error) {
	if err := admitModel(ctx, source); err != nil {
		return modeldata.Document{}, err
	}

	output := source

	output.Parts = make([]modeldata.Part, 0, len(source.Parts))
	for _, part := range source.Parts {
		if err := ctx.Err(); err != nil {
			return modeldata.Document{}, err
		}

		cleaned, err := cleanupPart(ctx, part)
		if err != nil {
			return modeldata.Document{}, err
		}

		if len(cleaned.Indices) > 0 {
			output.Parts = append(output.Parts, cleaned)
		}
	}

	if len(output.Parts) == 0 {
		return modeldata.Document{}, ErrInvalid
	}

	return output, nil
}

func cleanupPart(ctx context.Context, source modeldata.Part) (modeldata.Part, error) {
	output := source
	output.Vertices = nil
	output.Indices = nil
	unique := make(map[[meshdata.VertexStride]byte]uint32)

	for offset := 0; offset < len(source.Indices); offset += 12 {
		if offset%12288 == 0 {
			if err := ctx.Err(); err != nil {
				return modeldata.Part{}, err
			}
		}

		var triangle [3]uint32
		for corner := range triangle {
			triangle[corner] = binary.LittleEndian.Uint32(source.Indices[offset+corner*4:])
		}

		if degenerate(source.Vertices, triangle) {
			continue
		}

		for _, index := range triangle {
			vertex := source.Vertices[int(index)*meshdata.VertexStride : int(index+1)*meshdata.VertexStride]
			key := [meshdata.VertexStride]byte(vertex)

			remapped, exists := unique[key]
			if !exists {
				remapped = uint32(len(output.Vertices) / meshdata.VertexStride)
				unique[key] = remapped

				output.Vertices = append(output.Vertices, vertex...)
			}

			output.Indices = binary.LittleEndian.AppendUint32(output.Indices, remapped)
		}
	}

	return output, nil
}

func degenerate(vertices []byte, triangle [3]uint32) bool {
	var points [3][3]float64

	for corner, index := range triangle {
		for axis := range 3 {
			points[corner][axis] = float64(math.Float32frombits(binary.LittleEndian.Uint32(vertices[int(index)*32+axis*4:])))
		}
	}

	firstEdge, secondEdge := [3]float64{}, [3]float64{}
	for axis := range 3 {
		firstEdge[axis] = points[1][axis] - points[0][axis]
		secondEdge[axis] = points[2][axis] - points[0][axis]
	}

	return firstEdge[1]*secondEdge[2]-firstEdge[2]*secondEdge[1] == 0 &&
		firstEdge[2]*secondEdge[0]-firstEdge[0]*secondEdge[2] == 0 &&
		firstEdge[0]*secondEdge[1]-firstEdge[1]*secondEdge[0] == 0
}

// Optimize changes only triangle/vertex ordering within each independent part.
func Optimize(ctx context.Context, source modeldata.Document) (modeldata.Document, error) {
	model, _, err := OptimizeWithMappings(ctx, source)

	return model, err
}

// OptimizeWithMappings retains exact vertex remaps for resource-owned UV streams.
func OptimizeWithMappings(ctx context.Context, source modeldata.Document) (modeldata.Document, []Order, error) {
	if err := admitModel(ctx, source); err != nil {
		return modeldata.Document{}, nil, err
	}

	session, err := Open(ctx)
	if err != nil {
		return modeldata.Document{}, nil, err
	}

	defer func() { _ = session.Close(ctx) }()

	output := source
	orders := make([]Order, len(source.Parts))

	output.Parts = slices.Clone(source.Parts)
	for partIndex, part := range source.Parts {
		indices := make([]uint32, len(part.Indices)/4)
		for i := range indices {
			indices[i] = binary.LittleEndian.Uint32(part.Indices[i*4:])
		}

		order, err := session.Reorder(ctx, indices, len(part.Vertices)/meshdata.VertexStride)
		if err != nil {
			return modeldata.Document{}, nil, err
		}

		orders[partIndex] = order

		vertices := make([]byte, order.Vertices*meshdata.VertexStride)
		for original, destination := range order.Remap {
			if destination == math.MaxUint32 {
				continue
			}

			copy(vertices[int(destination)*32:], part.Vertices[original*32:(original+1)*32])
		}

		encoded := make([]byte, 0, len(part.Indices))
		for _, index := range order.Indices {
			encoded = binary.LittleEndian.AppendUint32(encoded, index)
		}

		output.Parts[partIndex].Vertices, output.Parts[partIndex].Indices = vertices, encoded
	}

	return output, orders, nil
}
