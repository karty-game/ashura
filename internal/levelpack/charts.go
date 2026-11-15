//go:build !js

// Package levelpack consolidates baked UV islands and compatible draw ranges
// offline. Bake-job dimensions never dictate the production draw boundaries.
//

package levelpack

import (
	"encoding/binary"
	"errors"
	"image"
	"math"

	"github.com/karty-game/karty-webgpu/internal/leveldata"
	"github.com/karty-game/karty-webgpu/internal/meshdata"
	"github.com/karty-game/karty-webgpu/internal/modeldata"
	"github.com/karty-game/karty-webgpu/internal/texturedata"
)

var ErrInput = errors.New("invalid offline lightmap consolidation")

// Input is one admitted bake page with external material identities.
type Input struct {
	Preview        bool
	Representation string
	Model          modeldata.Document
	UVs            [][]byte
	Materials      []leveldata.Material
	Maps           [3]texturedata.Data
	Scale          float32
	Encoding       string
}

type chart struct {
	input, part int
	indices     []uint32
	bounds      image.Rectangle
	position    image.Point
}

func charts(inputs []Input) ([]chart, error) {
	var output []chart

	for inputIndex, input := range inputs {
		if !validInput(input) {
			return nil, ErrInput
		}

		for partIndex, part := range input.Model.Parts {
			components, err := partCharts(part, input.UVs[partIndex], input.Maps[0].Width, input.Maps[0].Height)
			if err != nil {
				return nil, err
			}

			for _, component := range components {
				component.input, component.part = inputIndex, partIndex
				output = append(output, component)
			}
		}
	}

	return output, nil
}

func validInput(input Input) bool {
	if len(input.Model.Parts) == 0 || len(input.Model.Parts) != len(input.UVs) || len(input.Materials) != len(input.UVs) ||
		math.IsNaN(float64(input.Scale)) || math.IsInf(float64(input.Scale), 0) || input.Scale <= 0 ||
		input.Encoding != "rgbe8" && input.Encoding != "linear" {
		return false
	}

	for _, texture := range input.Maps[:input.MapCount()] {
		if texture.Width < 16 || texture.Height < 16 || texture.Width > 1024 || texture.Height > 1024 ||
			texture.Width != input.Maps[0].Width || texture.Height != input.Maps[0].Height ||
			texture.ColorSpace != texturedata.Linear || len(texture.Levels) != 1 ||
			len(texture.Levels[0]) != texture.Width*texture.Height*4 {
			return false
		}
	}

	return true
}

// xatlas duplicates vertices at chart seams. Connected indexed components thus
// never cross a UV seam; a disconnected subset may safely retain its own copy
// of the same lighting. Copy the surrounding four texels without resampling.
func partCharts(part modeldata.Part, coordinates []byte, width, height int) ([]chart, error) {
	if _, err := meshdata.FromOwned(part.Vertices, part.Indices); err != nil || len(coordinates) != len(part.Vertices)/4 {
		return nil, ErrInput
	}

	parent := make([]uint32, len(coordinates)/8)
	for index := range parent {
		parent[index] = uint32(index)
	}

	for offset := 0; offset < len(part.Indices); offset += 12 {
		first := root(parent, binary.LittleEndian.Uint32(part.Indices[offset:]))
		for corner := 1; corner < 3; corner++ {
			parent[root(parent, binary.LittleEndian.Uint32(part.Indices[offset+corner*4:]))] = first
		}
	}

	var output []chart

	byRoot := map[uint32]int{}

	for offset := 0; offset < len(part.Indices); offset += 4 {
		vertex := binary.LittleEndian.Uint32(part.Indices[offset:])
		component := root(parent, vertex)

		index, found := byRoot[component]
		if !found {
			index = len(output)
			byRoot[component] = index

			output = append(output, chart{bounds: image.Rectangle{Min: image.Pt(width, height)}})
		}

		value := &output[index]

		value.indices = append(value.indices, vertex)
		for axis, dimension := range []int{width, height} {
			coordinate := math.Float32frombits(binary.LittleEndian.Uint32(coordinates[int(vertex)*8+axis*4:]))
			if math.IsNaN(float64(coordinate)) || math.IsInf(float64(coordinate), 0) || coordinate < 0 || coordinate > 1 {
				return nil, ErrInput
			}

			low := max(0, int(math.Floor(float64(coordinate)*float64(dimension)))-4)

			high := min(dimension, int(math.Ceil(float64(coordinate)*float64(dimension)))+4)
			if axis == 0 {
				value.bounds.Min.X, value.bounds.Max.X = min(value.bounds.Min.X, low), max(value.bounds.Max.X, high)
			} else {
				value.bounds.Min.Y, value.bounds.Max.Y = min(value.bounds.Min.Y, low), max(value.bounds.Max.Y, high)
			}
		}
	}

	return output, nil
}

func root(parent []uint32, index uint32) uint32 {
	for parent[index] != index {
		parent[index] = parent[parent[index]]
		index = parent[index]
	}

	return index
}

// MapCount is the number of coefficient images in this output representation.
func (input Input) MapCount() int {
	if input.Representation == leveldata.RGBDiffuse {
		return 1
	}

	return len(input.Maps)
}
