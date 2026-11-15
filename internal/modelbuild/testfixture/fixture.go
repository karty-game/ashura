//go:build !js

// Package testfixture constructs engine-owned GLB sources for model validation.
// It is build-only and never enters runtime game dependencies.
//
//nolint:mnd,goconst,forcetypeassert,errcheck // Literal engine-owned fixture schema, numeric data and known JSON types remain visible.
package testfixture

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"math"

	"github.com/karty-game/karty-webgpu/pkg/mesh"
)

// Triangle returns an independently mutable glTF document and its BIN bytes.
func Triangle() (map[string]any, []byte) {
	positions := []float32{-.5, -.5, 0, .5, -.5, 0, 0, .5, 0}
	normals := []float32{0, 0, 1, 0, 0, 1, 0, 0, 1}
	uvs := []float32{0, 1, 1, 1, .5, 0}
	data := appendFloats(nil, positions)
	data = appendFloats(data, normals)

	data = appendFloats(data, uvs)
	for _, index := range []uint16{0, 1, 2} {
		data = binary.LittleEndian.AppendUint16(data, index)
	}

	document := map[string]any{
		"asset":   map[string]any{"version": "2.0"},
		"buffers": []any{map[string]any{"byteLength": len(data)}},
		"bufferViews": []any{
			map[string]any{"buffer": 0, "byteOffset": 0, "byteLength": 36},
			map[string]any{"buffer": 0, "byteOffset": 36, "byteLength": 36},
			map[string]any{
				"buffer":     0,
				"byteOffset": 72,
				"byteLength": 24,
			},
			map[string]any{"buffer": 0, "byteOffset": 96, "byteLength": 6},
		},
		"accessors": []any{
			map[string]any{
				"bufferView":    0,
				"componentType": 5126,
				"count":         3,
				"type":          "VEC3",
				"min":           []float64{-.5, -.5, 0},
				"max":           []float64{.5, .5, 0},
			},
			map[string]any{"bufferView": 1, "componentType": 5126, "count": 3, "type": "VEC3"},
			map[string]any{"bufferView": 2, "componentType": 5126, "count": 3, "type": "VEC2"},
			map[string]any{"bufferView": 3, "componentType": 5123, "count": 3, "type": "SCALAR"},
		},
		"materials": []any{
			map[string]any{
				"pbrMetallicRoughness": map[string]any{
					"metallicFactor":  0,
					"roughnessFactor": .6,
					"baseColorFactor": []float64{1, .7, .25, 1},
				},
			},
		},
		"meshes": []any{
			map[string]any{
				"primitives": []any{
					map[string]any{"attributes": map[string]any{"POSITION": 0, "NORMAL": 1, "TEXCOORD_0": 2}, "indices": 3, "material": 0},
				},
			},
		},
		"nodes":  []any{map[string]any{"mesh": 0}},
		"scenes": []any{map[string]any{"nodes": []int{0}}},
		"scene":  0,
	}

	return document, data
}

// GLB serializes JSON and BIN with their required independent padding.
//
//nolint:gosec // Owned validation fixtures have bounded literal payload sizes.
func GLB(document map[string]any, binaryData []byte) []byte {
	encoded, err := json.Marshal(document)
	if err != nil {
		panic(err)
	}

	for len(encoded)%4 != 0 {
		encoded = append(encoded, ' ')
	}

	binaryData = append([]byte(nil), binaryData...)
	for len(binaryData)%4 != 0 {
		binaryData = append(binaryData, 0)
	}

	data := []byte{'g', 'l', 'T', 'F'}
	for _, word := range []uint32{2, uint32(12 + 8 + len(encoded) + 8 + len(binaryData)), uint32(len(encoded)), 0x4e4f534a} {
		data = binary.LittleEndian.AppendUint32(data, word)
	}

	data = append(data, encoded...)
	data = binary.LittleEndian.AppendUint32(data, uint32(len(binaryData)))
	data = binary.LittleEndian.AppendUint32(data, 0x004e4942)

	return append(data, binaryData...)
}

// EmbedTexture appends a 2x2 PNG and assigns it as the first material's albedo.
func EmbedTexture(document map[string]any, data []byte) []byte {
	image := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	image.SetNRGBA(0, 0, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
	image.SetNRGBA(1, 0, color.NRGBA{R: 40, G: 180, B: 220, A: 255})
	image.SetNRGBA(0, 1, color.NRGBA{R: 40, G: 180, B: 220, A: 255})
	image.SetNRGBA(1, 1, color.NRGBA{R: 255, G: 255, B: 255, A: 255})

	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image); err != nil {
		panic(err)
	}

	for len(data)%4 != 0 {
		data = append(data, 0)
	}

	views := document["bufferViews"].([]any)
	view := len(views)
	document["bufferViews"] = append(views, map[string]any{"buffer": 0, "byteOffset": len(data), "byteLength": encoded.Len()})
	document["images"] = []any{map[string]any{"bufferView": view, "mimeType": "image/png"}}
	document["textures"] = []any{map[string]any{"source": 0}}
	document["materials"].([]any)[0].(map[string]any)["pbrMetallicRoughness"].(map[string]any)["baseColorTexture"] = map[string]any{
		"index": 0,
	}

	data = append(data, encoded.Bytes()...)
	document["buffers"].([]any)[0].(map[string]any)["byteLength"] = len(data)

	return data
}

// Scene builds a textured multi-material cube hierarchy including a mirrored
// node and a nonuniformly scaled/rotated child for maintained sample coverage.
func Scene() []byte {
	document, _ := Triangle()
	cube := mesh.NewBox("cube", mesh.BoxOptions{})

	positions := make([]float32, 0, 3*len(cube.Data.Positions))
	normals := make([]float32, 0, 3*len(cube.Data.Normals))

	uvs := make([]float32, 0, 2*len(cube.Data.UVs))
	for index, value := range cube.Data.Positions {
		positions = append(positions, value.X, value.Y, value.Z)
		normal := cube.Data.Normals[index]
		normals = append(normals, normal.X, normal.Y, normal.Z)
		uv := cube.Data.UVs[index]
		uvs = append(uvs, uv.X, uv.Y)
	}

	data := appendFloats(nil, positions)
	data = appendFloats(data, normals)

	data = appendFloats(data, uvs)
	for _, index := range cube.Data.Indices {
		data = binary.LittleEndian.AppendUint32(data, index)
	}

	cubeAccessors(document, len(data))
	data = EmbedTexture(document, data)
	document["materials"] = append(
		document["materials"].([]any),
		map[string]any{
			"pbrMetallicRoughness": map[string]any{
				"metallicFactor":  0,
				"roughnessFactor": .8,
				"baseColorFactor": []float64{.8, .2, .15, 1},
			},
			"doubleSided": true,
		},
	)
	document["meshes"] = append(
		document["meshes"].([]any),
		map[string]any{
			"primitives": []any{
				map[string]any{"attributes": map[string]any{"POSITION": 0, "NORMAL": 1, "TEXCOORD_0": 2}, "indices": 3, "material": 1},
			},
		},
	)
	document["nodes"] = []any{
		map[string]any{"name": "assembly", "children": []int{1, 2}, "translation": []float64{0, .1, 0}},
		map[string]any{"name": "textured", "mesh": 0, "translation": []float64{-.65, 0, 0}},
		map[string]any{
			"name":        "mirrored",
			"mesh":        1,
			"translation": []float64{.65, .3, 0},
			"scale":       []float64{-1, .65, 1},
			"rotation":    []float64{0, math.Sin(.2), 0, math.Cos(.2)},
		},
	}

	return GLB(document, data)
}

func appendFloats(data []byte, values []float32) []byte {
	for _, value := range values {
		data = binary.LittleEndian.AppendUint32(data, math.Float32bits(value))
	}

	return data
}

func cubeAccessors(document map[string]any, size int) {
	document["bufferViews"] = []any{
		map[string]any{"buffer": 0, "byteOffset": 0, "byteLength": 288},
		map[string]any{"buffer": 0, "byteOffset": 288, "byteLength": 288},
		map[string]any{
			"buffer":     0,
			"byteOffset": 576,
			"byteLength": 192,
		},
		map[string]any{"buffer": 0, "byteOffset": 768, "byteLength": 144},
	}

	accessors := document["accessors"].([]any)
	for index := range 3 {
		accessors[index].(map[string]any)["count"] = 24
	}

	accessors[0].(map[string]any)["min"], accessors[0].(map[string]any)["max"] = []float64{-.5, -.5, -.5}, []float64{.5, .5, .5}
	accessors[3].(map[string]any)["componentType"], accessors[3].(map[string]any)["count"] = 5125, 36
	document["buffers"].([]any)[0].(map[string]any)["byteLength"] = size
}
