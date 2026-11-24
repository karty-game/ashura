//go:build !js

//nolint:gosec,varnamelen // x/y/z iterate admitted int32 cell bounds before narrowing.
package levelbuild

import (
	"cmp"
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"slices"

	"github.com/karty-game/ashura/internal/meshdata"
	"github.com/karty-game/ashura/internal/modeldata"
)

type partition struct {
	options   Options
	spaces    map[string]*bucket
	chunks    map[cellKey]*bucket
	hashes    map[textureIdentity][32]byte
	input     uint64
	triangles uint64
}

// Split transforms static placements, clips triangles against exact grid planes,
// merges compatible draw ranges and prepares full per-space visibility geometry.
// Dynamic placements contribute neither merged receivers nor static occluders.
// No input geometry or authored material/UV data is changed.
func Split(ctx context.Context, placements []Placement, options Options) (Result, error) {
	if ctx == nil || len(placements) > 8192 {
		return Result{}, ErrGeometry
	}

	options, err := normalize(options)
	if err != nil {
		return Result{}, err
	}

	ordered := slices.Clone(placements)
	for index := range ordered {
		if ordered[index].Space == "" {
			ordered[index].Space = "default"
		}
	}

	slices.SortFunc(ordered, func(first, second Placement) int {
		if first.Space != second.Space {
			return cmp.Compare(first.Space, second.Space)
		}

		return cmp.Compare(first.ID, second.ID)
	})

	work := partition{options: options, spaces: make(map[string]*bucket), chunks: make(map[cellKey]*bucket),
		hashes: make(map[textureIdentity][32]byte)}

	for index, placement := range ordered {
		if index > 0 && placement.ID == ordered[index-1].ID && placement.Space == ordered[index-1].Space {
			return Result{}, fmt.Errorf("duplicate placement %s: %w", placement.ID, ErrGeometry)
		}

		if err = ctx.Err(); err != nil {
			return Result{}, err
		}

		if placement.Dynamic {
			continue
		}

		if err = work.placement(ctx, placement); err != nil {
			return Result{}, fmt.Errorf("placement %s: %w", placement.ID, err)
		}
	}

	return work.result(), nil
}

func (work *partition) placement(ctx context.Context, placement Placement) error {
	if err := validatePlacement(placement); err != nil {
		return err
	}

	transform, err := makeTransform(placement.Transform)
	if err != nil {
		return err
	}

	if work.spaces[placement.Space] == nil {
		work.spaces[placement.Space] = newBucket("level-space", placement.Space)
	}

	for _, original := range placement.Model.Parts {
		vertices := make([]vertex, len(original.Vertices)/meshdata.VertexStride)
		for index := range vertices {
			vertices[index], err = transform.vertex(original.Vertices[index*meshdata.VertexStride:])
			if err != nil {
				return err
			}
		}

		for offset := 0; offset < len(original.Indices); offset += 12 {
			if offset%3072 == 0 {
				if err = ctx.Err(); err != nil {
					return err
				}
			}

			value := triangle{}
			for corner := range 3 {
				value[corner] = vertices[binary.LittleEndian.Uint32(original.Indices[offset+corner*4:])]
			}

			if err = work.triangle(ctx, value, placement, original); err != nil {
				return err
			}
		}
	}

	return nil
}

func (work *partition) triangle(ctx context.Context, value triangle, placement Placement, original modeldata.Part) error {
	if areaSquared(value) == 0 {
		return nil
	}

	work.input++
	if work.input > uint64(work.options.MaxTriangles) {
		return ErrGeometry
	}

	if err := work.spaces[placement.Space].add(value, placement, original, work.hashes); err != nil {
		return err
	}

	minimum, maximum, err := cells(value, work.options)
	if err != nil {
		return err
	}

	for x := int64(minimum[0]); x <= int64(maximum[0]); x++ {
		for y := int64(minimum[1]); y <= int64(maximum[1]); y++ {
			for z := int64(minimum[2]); z <= int64(maximum[2]); z++ {
				if err = ctx.Err(); err != nil {
					return err
				}

				key := cellKey{space: placement.Space, cell: [3]int32{int32(x), int32(y), int32(z)}}
				if err = work.cell(value, placement, original, key); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func (work *partition) cell(value triangle, placement Placement, original modeldata.Part, key cellKey) error {
	minimum, maximum := cellBounds(key.cell, work.options)
	for axis := range 3 {
		if !finite(minimum[axis]) || !finite(maximum[axis]) || minimum[axis] >= maximum[axis] {
			return ErrGeometry
		}
	}

	polygon := clip(value, minimum, maximum)
	for corner := 1; corner+1 < len(polygon); corner++ {
		clipped := triangle{polygon[0], polygon[corner], polygon[corner+1]}
		for index := range clipped {
			for axis := range 3 {
				clipped[index].position[axis] = float64(float32(clipped[index].position[axis]))
			}
		}
		// Intersections use double precision, but the GPU payload uses float32.
		// Only discard triangles that are exactly degenerate in that payload;
		// small, representable triangles retain their geometry without an epsilon.
		if areaSquared(clipped) == 0 {
			continue
		}

		work.triangles++
		if work.triangles > uint64(work.options.MaxTriangles) {
			return ErrGeometry
		}

		output := work.chunks[key]
		if output == nil {
			if len(work.chunks) >= int(work.options.MaxChunks) {
				return ErrGeometry
			}

			for axis := range 3 {
				if math.Abs(minimum[axis]) > math.MaxFloat32 || math.Abs(maximum[axis]) > math.MaxFloat32 {
					return ErrGeometry
				}
			}

			output = newBucket("level-chunk", chunkID(key))
			work.chunks[key] = output
		}

		if err := output.add(clipped, placement, original, work.hashes); err != nil {
			return err
		}
	}

	return nil
}

func (work *partition) result() Result {
	result := Result{}

	for id, output := range work.spaces {
		if len(output.parts) > 0 {
			result.Spaces = append(result.Spaces, SpaceScene{ID: id, Model: output.document()})
		}
	}

	slices.SortFunc(result.Spaces, func(first, second SpaceScene) int { return cmp.Compare(first.ID, second.ID) })

	for key, output := range work.chunks {
		minimum, maximum := cellBounds(key.cell, work.options)

		chunk := Chunk{ID: chunkID(key), Space: key.space, Cell: key.cell, Model: output.document(), Sources: output.sources}
		for axis := range 3 {
			chunk.Origin[axis], chunk.Min[axis], chunk.Max[axis] = float32(minimum[axis]), float32(minimum[axis]), float32(maximum[axis])
		}

		result.Chunks = append(result.Chunks, chunk)
	}

	slices.SortFunc(result.Chunks, func(first, second Chunk) int {
		if first.Space != second.Space {
			return cmp.Compare(first.Space, second.Space)
		}

		for axis := range 3 {
			if first.Cell[axis] != second.Cell[axis] {
				return cmp.Compare(first.Cell[axis], second.Cell[axis])
			}
		}

		return 0
	})

	return result
}

func chunkID(key cellKey) string {
	return fmt.Sprintf("%s/%d/%d/%d", key.space, key.cell[0], key.cell[1], key.cell[2])
}
