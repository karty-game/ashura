//go:build !js

//nolint:mnd,gosec // Validated GPU layouts and atlas dimensions bound byte offsets and index conversions.
package levelpack

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"

	"github.com/karty-game/ashura/internal/leveldata"
	"github.com/karty-game/ashura/internal/meshdata"
	"github.com/karty-game/ashura/internal/modeldata"
)

// Output is ready for container encoding; its three maps have identical size.
type Output struct {
	Preview        bool
	Representation string
	Model          modeldata.Document
	UVs            [][]byte
	Materials      []leveldata.Material
	Maps           [3][]byte
	Size           int
	Scale          float32
	Encoding       string
}

func assemble(inputs []Input, value atlas) (Output, error) {
	family := inputs[value.charts[0].input]

	output := Output{
		Model:          modeldata.Document{Source: "consolidated:" + family.Model.Source},
		Size:           value.size,
		Scale:          family.Scale,
		Encoding:       family.Encoding,
		Representation: family.Representation,
	}
	for index := range family.MapCount() {
		output.Maps[index] = make([]byte, value.size*value.size*4)
	}

	materials := map[string]int{}

	var vertexCount, indexCount int

	for _, component := range value.charts {
		input := inputs[component.input]
		output.Preview = output.Preview || input.Preview
		material := input.Materials[component.part]

		key, err := json.Marshal(material)
		if err != nil {
			return Output{}, err
		}

		index, found := materials[string(key)]
		if !found {
			index = len(output.Model.Parts)
			if index >= modeldata.MaxParts {
				return Output{}, ErrInput
			}

			materials[string(key)] = index
			part := input.Model.Parts[component.part]
			part.Name = fmt.Sprintf("batch_%d", index)
			part.Vertices, part.Indices = nil, nil
			material.Name = part.Name
			output.Model.Parts = append(output.Model.Parts, part)
			output.Materials = append(output.Materials, material)
			output.UVs = append(output.UVs, nil)
		}

		part := &output.Model.Parts[index]
		before := len(part.Vertices)
		appendChart(part, &output.UVs[index], component, input, value.size)
		vertexCount += (len(part.Vertices) - before) / 32

		indexCount += len(component.indices)
		if vertexCount > meshdata.MaxVertices || indexCount > meshdata.MaxIndices {
			return Output{}, ErrInput
		}

		copyChartMaps(&output, component, input)
	}

	return output, nil
}

func appendChart(part *modeldata.Part, coordinates *[]byte, component chart, input Input, size int) {
	source := input.Model.Parts[component.part]
	sourceCoordinates := input.UVs[component.part]

	remap := map[uint32]uint32{}
	for _, original := range component.indices {
		index, found := remap[original]
		if !found {
			index = uint32(len(part.Vertices) / 32)
			remap[original] = index

			part.Vertices = append(part.Vertices, source.Vertices[int(original)*32:int(original+1)*32]...)
			for axis, dimension := range []int{input.Maps[0].Width, input.Maps[0].Height} {
				coordinate := math.Float32frombits(binary.LittleEndian.Uint32(sourceCoordinates[int(original)*8+axis*4:]))

				offset := component.position.X - component.bounds.Min.X
				if axis == 1 {
					offset = component.position.Y - component.bounds.Min.Y
				}

				coordinate = (coordinate*float32(dimension) + float32(offset)) / float32(size)
				*coordinates = binary.LittleEndian.AppendUint32(*coordinates, math.Float32bits(coordinate))
			}
		}

		part.Indices = binary.LittleEndian.AppendUint32(part.Indices, index)
	}
}

func copyChartMaps(output *Output, component chart, input Input) {
	for mapIndex, texture := range input.Maps[:input.MapCount()] {
		for row := range component.bounds.Dy() {
			source := ((component.bounds.Min.Y+row)*texture.Width + component.bounds.Min.X) * 4
			destination := ((component.position.Y+row)*output.Size + component.position.X) * 4
			copy(
				output.Maps[mapIndex][destination:destination+component.bounds.Dx()*4],
				texture.Levels[0][source:source+component.bounds.Dx()*4],
			)
		}
	}
}
