//go:build !js

package levelbuild

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"math"
	"testing"

	"github.com/karty-game/ashura/internal/modeldata"
)

func testModel(points [][3]float32, indices []uint32) modeldata.Document {
	part := modeldata.Part{Name: "floor", Material: modeldata.Material{Color: [4]float32{1, 1, 1, 1},
		Roughness: .5, Albedo: -1, Normal: -1, AO: -1}}

	for _, point := range points {
		for _, value := range []float32{point[0], point[1], point[2], 0, 1, 0, (point[0] + 1) / 2, (point[2] + 1) / 2} {
			part.Vertices = binary.LittleEndian.AppendUint32(part.Vertices, math.Float32bits(value))
		}
	}

	for _, index := range indices {
		part.Indices = binary.LittleEndian.AppendUint32(part.Indices, index)
	}

	return modeldata.Document{Source: "fixture", Parts: []modeldata.Part{part}}
}

func floorModel() modeldata.Document {
	return testModel([][3]float32{{-1, 0, -1}, {-1, 0, 1}, {1, 0, 1}, {1, 0, -1}}, []uint32{0, 1, 2, 0, 2, 3})
}

func readVertex(part modeldata.Part, index uint32) vertex {
	data := part.Vertices[index*32:]

	var value vertex
	for axis := range 3 {
		value.position[axis] = float64(math.Float32frombits(binary.LittleEndian.Uint32(data[axis*4:])))
		value.normal[axis] = float64(math.Float32frombits(binary.LittleEndian.Uint32(data[12+axis*4:])))
	}

	value.uv = [2]float64{float64(math.Float32frombits(binary.LittleEndian.Uint32(data[24:]))),
		float64(math.Float32frombits(binary.LittleEndian.Uint32(data[28:])))}

	return value
}

func triangleArea(document modeldata.Document) float64 {
	var result float64

	for _, part := range document.Parts {
		for offset := 0; offset < len(part.Indices); offset += 12 {
			var value triangle
			for corner := range 3 {
				value[corner] = readVertex(part, binary.LittleEndian.Uint32(part.Indices[offset+corner*4:]))
			}

			result += math.Sqrt(areaSquared(value)) / 2
		}
	}

	return result
}

func TestClippingPreservesAreaAttributesAndBoundaryIdentity(t *testing.T) {
	t.Parallel()

	result, err := Split(t.Context(), []Placement{{ID: "/floor", Model: floorModel()}}, Options{Size: [3]float32{1, 1, 1}})
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Chunks) != 4 || len(result.Spaces) != 1 || triangleArea(result.Spaces[0].Model) != 4 {
		t.Fatalf("unexpected partition: %d chunks, %d spaces", len(result.Chunks), len(result.Spaces))
	}

	var area float64

	edges := [2]map[[8]uint32]bool{{}, {}}

	for _, chunk := range result.Chunks {
		area += triangleArea(chunk.Model)
		checkChunkAttributes(t, chunk, edges)
	}

	if area != 4 || len(edges[0]) == 0 || len(edges[0]) != len(edges[1]) {
		t.Fatalf("area or shared edge changed: %g, %v", area, edges)
	}

	for value := range edges[0] {
		if !edges[1][value] {
			t.Fatal("adjacent cells disagree on encoded boundary vertex")
		}
	}
}

func checkChunkAttributes(t *testing.T, chunk Chunk, edges [2]map[[8]uint32]bool) {
	t.Helper()

	for _, part := range chunk.Model.Parts {
		for index := range len(part.Vertices) / 32 {
			value := readVertex(part, uint32(index))
			for axis := range 3 {
				if value.position[axis] < float64(chunk.Min[axis]) || value.position[axis] > float64(chunk.Max[axis]) {
					t.Fatalf("vertex outside chunk %s: %v", chunk.ID, value.position)
				}
			}

			if value.normal != (vector{0, 1, 0}) || value.uv != ([2]float64{(value.position[0] + 1) / 2, (value.position[2] + 1) / 2}) {
				t.Fatalf("lost UV0 or normal: %+v", value)
			}

			if value.position[0] == 0 {
				side := 0
				if chunk.Cell[0] >= 0 {
					side = 1
				}

				edges[side][encodeVertex(value)] = true
			}
		}
	}
}

func TestCoplanarBoundaryHasExactlyOneOwner(t *testing.T) {
	t.Parallel()

	model := testModel([][3]float32{{0, 1, 0}, {0, 1, 1}, {1, 1, 0}}, []uint32{0, 1, 2})

	result, err := Split(t.Context(), []Placement{{ID: "plane", Model: model}}, Options{Size: [3]float32{1, 1, 1}})
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Chunks) != 1 || result.Chunks[0].Cell != ([3]int32{0, 1, 0}) || triangleArea(result.Chunks[0].Model) != .5 {
		t.Fatalf("coplanar triangle duplicated or misowned: %+v", result.Chunks)
	}
}

func TestFloat32CollapsedBoundaryFragmentsDoNotBecomeGeometry(t *testing.T) {
	t.Parallel()

	model := testModel([][3]float32{{.25, 0, 0}, {.3, 0, 0}, {.3, 0, 1}}, []uint32{0, 1, 2})

	result, err := Split(t.Context(), []Placement{{ID: "rounding", Model: model}}, Options{Size: [3]float32{.1, 2, 2}})
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Chunks) != 1 || result.Chunks[0].Cell[0] != 2 || triangleArea(result.Chunks[0].Model) != triangleArea(model) {
		t.Fatalf(
			"encoded boundary partition chunks=%d cell=%v area=%g original=%g",
			len(result.Chunks),
			result.Chunks[0].Cell,
			triangleArea(result.Chunks[0].Model),
			triangleArea(model),
		)
	}
}

func TestMaterialsDeduplicateButReplacementGroupsStaySeparate(t *testing.T) {
	t.Parallel()

	model := floorModel()
	model.Textures = []modeldata.Texture{{Role: modeldata.Albedo, Data: []byte{1, 2, 3}},
		{Role: modeldata.Albedo, Data: []byte{1, 2, 3}}}
	model.Parts[0].Material.Albedo = 0
	second := model.Parts[0]
	second.Name, second.Material.Albedo = "second", 1
	model.Parts = append(model.Parts, second)
	placement := Placement{ID: "two-parts", Model: model}

	result, err := Split(t.Context(), []Placement{placement}, Options{Size: [3]float32{10, 10, 10}, Origin: [3]float32{-5, -5, -5}})
	if err != nil {
		t.Fatal(err)
	}

	chunk := result.Chunks[0]
	if len(chunk.Model.Textures) != 1 || len(chunk.Model.Parts) != 1 || len(chunk.Sources) != 2 || chunk.Sources[1].Name != "second" {
		t.Fatalf("material/texture/provenance merge mismatch: %+v", chunk)
	}

	placement.PartGroups = map[string]string{"floor": "brick-ssbump", "second": "stone-ssbump"}

	result, err = Split(t.Context(), []Placement{placement}, Options{Size: [3]float32{10, 10, 10}, Origin: [3]float32{-5, -5, -5}})
	if err != nil || len(result.Chunks[0].Model.Parts) != 2 {
		t.Fatalf("external material override merged: %v", err)
	}

	other := Placement{ID: "replaceable", Group: "impostor-group", Model: model}

	result, err = Split(t.Context(), []Placement{other, {ID: "permanent", Model: model}},
		Options{Size: [3]float32{10, 10, 10}, Origin: [3]float32{-5, -5, -5}})
	if err != nil || len(result.Chunks[0].Model.Parts) != 2 {
		t.Fatalf("independently switchable group merged: %v", err)
	}
}

func TestSpacesDynamicExclusionAndOrdering(t *testing.T) {
	t.Parallel()

	placements := []Placement{{ID: "fixed", Space: "room-b", Model: floorModel()},
		{ID: "fixed", Space: "room-a", Model: floorModel()}, {ID: "door", Dynamic: true}}

	first, err := Split(t.Context(), placements, Options{})
	if err != nil {
		t.Fatal(err)
	}

	second, err := Split(t.Context(), []Placement{placements[2], placements[1], placements[0]}, Options{})
	if err != nil || len(first.Spaces) != 2 || len(first.Chunks) != len(second.Chunks) {
		t.Fatalf("space ownership/dynamic exclusion: %v", err)
	}

	for index := range first.Chunks {
		left, leftErr := modeldata.Encode(first.Chunks[index].Model)

		right, rightErr := modeldata.Encode(second.Chunks[index].Model)
		if leftErr != nil || rightErr != nil || !bytes.Equal(left, right) || first.Chunks[index].ID != second.Chunks[index].ID {
			t.Fatalf("placement ordering changes output %d: %v, %v", index, leftErr, rightErr)
		}
	}
}

func TestNonuniformScaleUsesInverseTransposeNormals(t *testing.T) {
	t.Parallel()

	model := testModel([][3]float32{{0, 0, 0}, {0, 0, 1}, {1, 1, 0}}, []uint32{0, 1, 2})
	for offset := 0; offset < len(model.Parts[0].Vertices); offset += 32 {
		binary.LittleEndian.PutUint32(model.Parts[0].Vertices[offset+12:], math.Float32bits(-1/float32(math.Sqrt2)))
		binary.LittleEndian.PutUint32(model.Parts[0].Vertices[offset+16:], math.Float32bits(1/float32(math.Sqrt2)))
	}

	placement := Placement{ID: "scaled", Model: model, Transform: [16]float32{2, 0, 0, 0, 0, 1, 0, 0, 0, 0, 3, 0, 5, 7, 9, 1}}

	result, err := Split(t.Context(), []Placement{placement}, Options{})
	if err != nil {
		t.Fatal(err)
	}

	normal := readVertex(result.Spaces[0].Model.Parts[0], 0).normal
	if math.Abs(normal[0]+1/math.Sqrt(5)) > 1e-6 || math.Abs(normal[1]-2/math.Sqrt(5)) > 1e-6 || normal[2] != 0 {
		t.Fatalf("normal transformed like position: %v", normal)
	}
}

func TestPartitionRejectsUnboundedAndInvalidInputs(t *testing.T) {
	t.Parallel()

	placement := Placement{ID: "floor", Model: floorModel()}

	for name, options := range map[string]Options{
		"tiny cell budget": {Size: [3]float32{1, 1, 1}, MaxChunks: 1},
		"triangle budget":  {MaxTriangles: 1},
		"nonfinite":        {Size: [3]float32{float32(math.NaN()), 1, 1}},
		"negative size":    {Size: [3]float32{1, -1, 1}},
		"oversized cap":    {MaxChunks: MaxChunks + 1},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if _, err := Split(t.Context(), []Placement{placement}, options); err == nil {
				t.Fatal("invalid partition admitted")
			}
		})
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := Split(ctx, []Placement{placement}, Options{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel ignored: %v", err)
	}

	placement.Transform = [16]float32{-1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}
	if _, err := Split(t.Context(), []Placement{placement}, Options{}); err == nil {
		t.Fatal("negative-determinant placement accepted")
	}
}
