package impostordata

import (
	"encoding/binary"
	"math"
)

func metadata(document Document) [metadataWordCount]uint32 {
	return [metadataWordCount]uint32{
		math.Float32bits(document.Center[0]),
		math.Float32bits(document.Center[1]),
		math.Float32bits(document.Center[2]),
		math.Float32bits(
			document.Radius,
		),
		math.Float32bits(document.Roughness),
		math.Float32bits(document.ElevationMin),
		math.Float32bits(document.ElevationMax),
		document.Azimuths,
		document.Elevations,
		document.AtlasWidth,
		document.AtlasHeight,
		document.Columns,
		document.CellSize,
		document.Resolution,
		document.Padding,
		document.MipLevels,
		document.SourceTriangles,
		uint32(
			document.Mode,
		),
		math.Float32bits(document.HalfExtents[0]),
		math.Float32bits(document.HalfExtents[1]),
		math.Float32bits(document.HalfExtents[2]),
		uint32(document.Storage),
	}
}
func fromMetadata(words [metadataWordCount]uint32) Document {
	return Document{
		Center: [3]float32{math.Float32frombits(words[0]), math.Float32frombits(words[1]), math.Float32frombits(words[2])},
		Radius: math.Float32frombits(
			words[3],
		),
		Roughness:       math.Float32frombits(words[4]),
		ElevationMin:    math.Float32frombits(words[5]),
		ElevationMax:    math.Float32frombits(words[6]),
		Azimuths:        words[7],
		Elevations:      words[8],
		AtlasWidth:      words[9],
		AtlasHeight:     words[10],
		Columns:         words[11],
		CellSize:        words[12],
		Resolution:      words[13],
		Padding:         words[14],
		MipLevels:       words[15],
		SourceTriangles: words[16],
		Mode: Mode(
			words[17],
		),
		HalfExtents: [3]float32{math.Float32frombits(words[18]), math.Float32frombits(words[19]), math.Float32frombits(words[20])},
		Storage:     Storage(words[21]),
	}
}

func uniformHeader(document Document) [24]float32 {
	return [24]float32{
		document.Center[0], document.Center[1], document.Center[2], document.Radius,
		float32(document.AtlasWidth), float32(document.AtlasHeight), float32(document.CellSize), float32(document.Resolution),
		float32(document.Columns), float32(document.Padding), float32(document.Azimuths), float32(document.Elevations),
		document.ElevationMin, document.ElevationMax, document.Roughness, alphaCutoff,
		document.HalfExtents[0], document.HalfExtents[1], document.HalfExtents[2], float32(document.Mode),
		float32(document.Storage), 0, 0, 0,
	}
}

// bakeUniform constructs the GPU-ready table during offline serialization.
func bakeUniform(document Document) []byte {
	data := make([]byte, UniformSize)
	for index, value := range uniformHeader(document) {
		binary.LittleEndian.PutUint32(data[index*4:], math.Float32bits(value))
	}

	for index, view := range document.Views[:min(len(document.Views), MaxViews)] {
		for axisIndex, axis := range [][3]float32{view.Right, view.Up, view.Outward} {
			for component, value := range axis {
				binary.LittleEndian.PutUint32(data[uniformHeaderBytes+index*48+axisIndex*16+component*4:], math.Float32bits(value))
			}
		}

		binary.LittleEndian.PutUint32(data[uniformHeaderBytes+index*48+12:], math.Float32bits(view.PlaneDepth))
	}

	return data
}

// validateUniform checks prepared bytes in place; it never repacks them at load.
func validateUniform(document Document) error {
	for index, value := range uniformHeader(document) {
		if binary.LittleEndian.Uint32(document.Uniform[index*4:]) != math.Float32bits(value) {
			return errFormat
		}
	}

	for index := range MaxViews {
		var view View
		if index < len(document.Views) {
			view = document.Views[index]
		}

		for axisIndex, axis := range [][3]float32{view.Right, view.Up, view.Outward} {
			for component := range 4 {
				value := float32(0)
				if component < len(axis) {
					value = axis[component]
				}

				if component == 3 && axisIndex == 0 {
					value = view.PlaneDepth
				}

				if binary.LittleEndian.Uint32(
					document.Uniform[uniformHeaderBytes+index*48+axisIndex*16+component*4:],
				) != math.Float32bits(
					value,
				) {
					return errFormat
				}
			}
		}
	}

	return nil
}
