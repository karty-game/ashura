//go:build !js

//nolint:gosec // Existing model limits bound vertex and triangle remap conversions.
package meshopt

import (
	"context"
	"encoding/binary"
	"math"

	"github.com/karty-game/karty-webgpu/internal/modeldata"
)

// OptimizeUV remaps an independent UV1 stream and global old-to-new triangle
// references with the geometry. It never charts, clips or bakes again.
func OptimizeUV(ctx context.Context, source modeldata.Document, uvs [][]byte) (modeldata.Document, [][]byte, []uint32, error) {
	if len(uvs) != len(source.Parts) {
		return modeldata.Document{}, nil, nil, ErrInvalid
	}

	for index, part := range source.Parts {
		if len(uvs[index]) != len(part.Vertices)/4 {
			return modeldata.Document{}, nil, nil, ErrInvalid
		}
	}

	model, orders, err := OptimizeWithMappings(ctx, source)
	if err != nil {
		return modeldata.Document{}, nil, nil, err
	}

	streams := make([][]byte, len(uvs))

	var (
		triangles []uint32
		base      uint32
	)

	for index, order := range orders {
		streams[index] = make([]byte, order.Vertices*8)
		for original, destination := range order.Remap {
			if destination != math.MaxUint32 {
				copy(streams[index][int(destination)*8:], uvs[index][original*8:(original+1)*8])
			}
		}

		remap, err := triangleRemap(source.Parts[index].Indices, order)
		if err != nil {
			return modeldata.Document{}, nil, nil, err
		}

		for _, destination := range remap {
			triangles = append(triangles, base+destination)
		}

		base += uint32(len(remap))
	}

	return model, streams, triangles, nil
}

func triangleRemap(indices []byte, order Order) ([]uint32, error) {
	const triangleBytes = 3 * 4

	count := len(indices) / triangleBytes
	if len(order.Indices) != count*3 {
		return nil, ErrInvalid
	}
	// Linked duplicate lists retain multiplicity and original corner orientation.
	heads := make(map[[3]uint32]uint32, count)

	next := make([]uint32, count)
	for triangle := count - 1; triangle >= 0; triangle-- {
		var face [3]uint32
		for corner := range 3 {
			face[corner] = binary.LittleEndian.Uint32(indices[triangle*12+corner*4:])
		}

		head, present := heads[face]
		if !present {
			head = math.MaxUint32
		}

		next[triangle] = head
		heads[face] = uint32(triangle)
	}

	inverse := make([]uint32, order.Vertices)
	for original, destination := range order.Remap {
		if destination != math.MaxUint32 {
			inverse[destination] = uint32(original)
		}
	}

	remap := make([]uint32, count)
	for triangle := range count {
		face := [3]uint32{inverse[order.Indices[triangle*3]], inverse[order.Indices[triangle*3+1]], inverse[order.Indices[triangle*3+2]]}

		original, present := heads[face]
		if !present || original == math.MaxUint32 {
			return nil, ErrInvalid
		}

		remap[original] = uint32(triangle)
		heads[face] = next[original]
	}

	return remap, nil
}
