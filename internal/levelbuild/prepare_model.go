//go:build !js

package levelbuild

import (
	"encoding/binary"
	"math"
	"slices"

	"github.com/karty-game/karty-webgpu/internal/meshdata"
	"github.com/karty-game/karty-webgpu/internal/modeldata"
)

// PrepareModel applies a static placement offline, preserving indices, UVs and
// materials. The result uses world-space vertices and an identity runtime pose.
func PrepareModel(input modeldata.Document, world [16]float32) (modeldata.Document, error) {
	if len(input.Parts) == 0 || len(input.Parts) > modeldata.MaxParts {
		return modeldata.Document{}, ErrGeometry
	}

	mapping, err := makeTransform(world)
	if err != nil {
		return modeldata.Document{}, err
	}

	output := input

	output.Parts = slices.Clone(input.Parts)
	for index := range output.Parts {
		part := &output.Parts[index]
		if _, err := meshdata.FromOwned(part.Vertices, part.Indices); err != nil {
			return modeldata.Document{}, err
		}

		part.Vertices = slices.Clone(part.Vertices)
		for offset := 0; offset < len(part.Vertices); offset += 32 {
			vertex, err := mapping.vertex(part.Vertices[offset:])
			if err != nil {
				return modeldata.Document{}, err
			}

			for axis := range 3 {
				binary.LittleEndian.PutUint32(part.Vertices[offset+axis*4:], math.Float32bits(float32(vertex.position[axis])))
				binary.LittleEndian.PutUint32(part.Vertices[offset+12+axis*4:], math.Float32bits(float32(vertex.normal[axis])))
			}
		}
	}

	return output, nil
}
