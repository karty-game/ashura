//go:build !js

//nolint:mnd // The fixed GLB header, accessor component types and source colors follow the source format and artwork.
package impostorfixture

import (
	"encoding/binary"
	"encoding/json"
	"math"
)

type stream struct {
	data      []byte
	views     []map[string]any
	accessors []map[string]any
}

func encode(parts []part) ([]byte, Stats, error) {
	output := stream{}
	primitives := make([]map[string]any, 0, len(parts))
	stats := Stats{
		Primitives: len(parts),
		Min:        [3]float32{math.MaxFloat32, math.MaxFloat32, math.MaxFloat32},
		Max:        [3]float32{-math.MaxFloat32, -math.MaxFloat32, -math.MaxFloat32},
	}

	for index, value := range parts {
		positions := output.floats(value.positions)
		normals := output.floats(value.normals)
		indices := output.indices(value.indices)
		primitives = append(
			primitives,
			map[string]any{"attributes": map[string]int{"POSITION": positions, "NORMAL": normals}, "indices": indices, "material": index},
		)
		stats.Triangles += len(value.indices) / 3
		stats.Vertices += len(value.positions) / 3

		for offset, coordinate := range value.positions {
			axis := offset % 3
			stats.Min[axis] = min(stats.Min[axis], coordinate)
			stats.Max[axis] = max(stats.Max[axis], coordinate)
		}
	}

	palette := [][4]float32{{.16, .07, .025, 1}, {.28, .13, .045, 1}, {.045, .17, .025, 1}, {.15, .32, .045, 1}}
	materials := make([]map[string]any, 0, len(palette))

	for _, color := range palette {
		materials = append(
			materials,
			map[string]any{"pbrMetallicRoughness": map[string]any{"baseColorFactor": color, "metallicFactor": 0, "roughnessFactor": .85}},
		)
	}

	document := map[string]any{
		"asset":       map[string]any{"version": "2.0", "generator": "Karty procedural gnarled tree"},
		"buffers":     []map[string]any{{"byteLength": len(output.data)}},
		"bufferViews": output.views, "accessors": output.accessors, "materials": materials,
		"meshes": []map[string]any{{"name": "Gnarled tree", "primitives": primitives}},
		"nodes":  []map[string]any{{"name": "Gnarled tree", "mesh": 0}},
		"scenes": []map[string]any{{"nodes": []int{0}}}, "scene": 0,
	}
	data, err := glb(document, output.data)

	return data, stats, err
}

func (output *stream) floats(values []float32) int {
	start := len(output.data)
	minimum := [3]float32{math.MaxFloat32, math.MaxFloat32, math.MaxFloat32}
	maximum := [3]float32{-math.MaxFloat32, -math.MaxFloat32, -math.MaxFloat32}

	for index, value := range values {
		axis := index % 3
		minimum[axis], maximum[axis] = min(minimum[axis], value), max(maximum[axis], value)
	}

	for _, value := range values {
		output.data = binary.LittleEndian.AppendUint32(output.data, math.Float32bits(value))
	}

	index := output.accessor(start, len(values)*4, len(values)/3, 5126, "VEC3")
	output.accessors[index]["min"], output.accessors[index]["max"] = minimum, maximum

	return index
}

func (output *stream) indices(values []uint32) int {
	start := len(output.data)
	for _, value := range values {
		output.data = binary.LittleEndian.AppendUint32(output.data, value)
	}

	return output.accessor(start, len(values)*4, len(values), 5125, "SCALAR")
}

func (output *stream) accessor(offset, length, count, component int, kind string) int {
	view, index := len(output.views), len(output.accessors)
	output.views = append(output.views, map[string]any{"buffer": 0, "byteOffset": offset, "byteLength": length})
	output.accessors = append(
		output.accessors,
		map[string]any{"bufferView": view, "componentType": component, "count": count, "type": kind},
	)

	return index
}

//nolint:gosec // This fixed fixture is well below the 64 MiB source bound and each length fits uint32.
func glb(document map[string]any, data []byte) ([]byte, error) {
	encoded, err := json.Marshal(document)
	if err != nil {
		return nil, err
	}

	for len(encoded)%4 != 0 {
		encoded = append(encoded, ' ')
	}

	for len(data)%4 != 0 {
		data = append(data, 0)
	}

	result := []byte{'g', 'l', 'T', 'F'}
	for _, word := range []uint32{2, uint32(12 + 8 + len(encoded) + 8 + len(data)), uint32(len(encoded)), 0x4e4f534a} {
		result = binary.LittleEndian.AppendUint32(result, word)
	}

	result = append(result, encoded...)
	result = binary.LittleEndian.AppendUint32(result, uint32(len(data)))
	result = binary.LittleEndian.AppendUint32(result, 0x004e4942)

	return append(result, data...), nil
}
