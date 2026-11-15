//go:build !js

package lightmapbake

import (
	"encoding/binary"
	"math"
	"reflect"
	"testing"

	"github.com/karty-game/karty-webgpu/internal/modeldata"
	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
)

//nolint:gocognit // This regression checks geometry admission and both exact secondary-UV receiver supports together.
func TestModelVisibilityCullsOnlyExactZeroAreaAndPreservesReceiverBinding(t *testing.T) {
	t.Parallel()

	positions := []Vec3{
		{0, 0, 0}, {1, 0, 0}, {2, 0, 0},
		{10, 0, 0}, {11, 0, 0}, {10, 1, 0},
		{20, 0, 0}, {21, 0, 0}, {20, 1e-9, 0},
	}
	coordinates := []Vec2{{0, 0}, {1, 0}, {0, 1}, {0, 0}, {1, 0}, {0, 1}, {0, 0}, {1, 0}, {0, 1}}
	part := modeldata.Part{
		Name: "surface", Vertices: modelVertices(positions, coordinates),
		Indices: modelIndices(0, 1, 2, 3, 4, 5, 6, 7, 8), Material: modelMaterial(),
	}
	data := encodeModelFixture(t, modeldata.Document{Source: "binding", Parts: []modeldata.Part{part}})

	ordered, err := sceneFromModel(data, "room", 1)
	if err != nil {
		t.Fatal(err)
	}

	if len(ordered.Triangles) != 3 {
		t.Fatal("raw conversion changed secondary-UV triangle indices", len(ordered.Triangles))
	}

	for index, triangle := range ordered.Triangles {
		if triangle.ID != uint64(index)+1 || triangle.Positions[0] != positions[index*3] ||
			triangle.UV != [3]Vec2{coordinates[index*3], coordinates[index*3+1], coordinates[index*3+2]} {
			t.Fatal("raw geometry/UV0 binding changed", index, triangle)
		}
	}

	visible, err := SceneFromModel(data, "room", 1)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(visible.Triangles, ordered.Triangles[1:]) {
		t.Fatal("visibility conversion removed positive thin geometry or changed ordering", visible.Triangles)
	}

	if _, err = PrepareScene(visible); err != nil {
		t.Fatal("positive thin visibility geometry must remain admissible", err)
	}

	secondary := modelUVBytes([]Vec2{
		{0, 0}, {0, 0}, {0, 0},
		{.05, .05}, {.45, .05}, {.05, .45},
		{.55, .55}, {.95, .55}, {.55, .95},
	})

	receivers, err := ReceiversFromModel(data, [][]byte{secondary}, 16, 16)
	if err != nil {
		t.Fatal("receiver UV1 binding shifted after visibility culling", err)
	}

	var ordinary, thin int

	for _, receiver := range receivers {
		if receiver.Coverage <= 0 || len(receiver.Patches) == 0 {
			t.Fatal("missing covered world support", receiver)
		}

		origin := float32(10)
		if receiver.Pixel%16 >= 8 {
			origin = 20
			thin++
		} else {
			ordinary++
		}

		for _, patch := range receiver.Patches {
			if patch.Triangle != math.MaxUint32 || patch.GeometricNormal[2] < .99 {
				t.Fatal("invalid independent receiver frame", patch)
			}

			for _, position := range patch.Positions {
				if position[0] < origin-1e-4 || position[0] > origin+1+1e-4 {
					t.Fatal("UV1 chart bound to a different world triangle", receiver.Pixel, position)
				}
			}
		}
	}

	if ordinary == 0 || thin == 0 {
		t.Fatal("lost ordinary or positive thin receiver support", ordinary, thin)
	}
}

func TestModelTransportIncludesOnlyReferencedAlbedoTextures(t *testing.T) {
	t.Parallel()

	material := modelMaterial()
	material.Albedo, material.Normal, material.AO = 1, 0, 2
	document := modeldata.Document{
		Source: "role-filter", Parts: []modeldata.Part{modelTriangle("surface", 0, material)},
		Textures: []modeldata.Texture{
			{Role: modeldata.Normal, Data: modelTexture(t, assetbuild.UsageNormal, [4]byte{128, 128, 255, 255})},
			{Role: modeldata.Albedo, Data: modelTexture(t, assetbuild.UsageAlbedo, [4]byte{128, 64, 255, 255})},
			{Role: modeldata.Occlusion, Data: modelTexture(t, assetbuild.UsageAO, [4]byte{50, 50, 50, 255})},
			{Role: modeldata.Albedo, Data: modelTexture(t, assetbuild.UsageAlbedo, [4]byte{255, 0, 0, 255})},
		},
	}

	scene, err := SceneFromModel(encodeModelFixture(t, document), "room", 1)
	if err != nil {
		t.Fatal(err)
	}

	if len(scene.Textures) != 1 || scene.Materials[0].Texture != 1 {
		t.Fatal("non-transport textures retained or albedo index not remapped", scene.Materials, len(scene.Textures))
	}

	pixel := scene.Textures[0].Pixels[0]
	if math.Abs(float64(pixel[0])-.2158605) > 1e-6 || math.Abs(float64(pixel[1])-.05126946) > 1e-6 || pixel[2] != 1 {
		t.Fatal("albedo not decoded to linear transport reflectance", pixel)
	}
}

func TestModelReceiverMaterialIdentitySurvivesIndependentChunkBatching(t *testing.T) {
	t.Parallel()

	red := modeldata.Texture{Role: modeldata.Albedo, Data: modelTexture(t, assetbuild.UsageAlbedo, [4]byte{255, 0, 0, 255})}
	blue := modeldata.Texture{Role: modeldata.Albedo, Data: modelTexture(t, assetbuild.UsageAlbedo, [4]byte{0, 0, 255, 255})}
	redMaterial, blueMaterial := modelMaterial(), modelMaterial()
	redMaterial.Albedo = 0
	blueMaterial.Albedo, blueMaterial.DoubleSided = 1, true
	visibility := modeldata.Document{
		Source: "visibility", Textures: []modeldata.Texture{red, blue},
		Parts: []modeldata.Part{modelTriangle("batch_0", 0, redMaterial), modelTriangle("batch_1", 10, blueMaterial)},
	}

	source, err := SceneFromModel(encodeModelFixture(t, visibility), "room", 1)
	if err != nil {
		t.Fatal(err)
	}

	prepared, err := PrepareScene(source)
	if err != nil {
		t.Fatal(err)
	}

	for _, back := range []bool{false, true} {
		t.Run(map[bool]string{false: "subset", true: "back-side-subset"}[back], func(t *testing.T) {
			t.Parallel()

			material := blueMaterial
			material.Albedo, material.DoubleSided = 0, false

			part := modelTriangle("batch_0", 10, material)
			if back {
				part.Name += "@back"

				part.Indices = modelIndices(0, 2, 1)
				for offset := 20; offset < len(part.Vertices); offset += 32 {
					binary.LittleEndian.PutUint32(part.Vertices[offset:], math.Float32bits(-1))
				}
			}

			document := modeldata.Document{Source: "subset", Parts: []modeldata.Part{part}, Textures: []modeldata.Texture{blue, red}}
			data := encodeModelFixture(t, document)

			receivers := modelReceivers(t, data, [][]byte{modelUVBytes([]Vec2{{.05, .05}, {.45, .05}, {.05, .45}})})
			if err := RemapReceiverMaterials(data, prepared, receivers); err != nil {
				t.Fatal(err)
			}

			assertReceiverMaterial(t, receivers, 1)
		})
	}

	t.Run("reordered-parts-and-textures", func(t *testing.T) {
		t.Parallel()

		first, second := blueMaterial, redMaterial
		first.Albedo, second.Albedo = 0, 1
		document := modeldata.Document{
			Source: "reordered", Textures: []modeldata.Texture{blue, red},
			Parts: []modeldata.Part{modelTriangle("batch_0", 10, first), modelTriangle("batch_1", 0, second)},
		}
		data := encodeModelFixture(t, document)

		receivers := modelReceivers(t, data, [][]byte{
			modelUVBytes([]Vec2{{.05, .05}, {.45, .05}, {.05, .45}}),
			modelUVBytes([]Vec2{{.55, .55}, {.95, .55}, {.55, .95}}),
		})
		if err := RemapReceiverMaterials(data, prepared, receivers); err != nil {
			t.Fatal(err)
		}

		for _, receiver := range receivers {
			var expected uint32
			if receiver.Pixel%16 < 8 {
				expected = 1
			}

			assertReceiverMaterial(t, []Receiver{receiver}, expected)
		}
	})
}

func modelMaterial() modeldata.Material {
	return modeldata.Material{Color: [4]float32{1, 1, 1, 1}, Roughness: .5, Albedo: -1, Normal: -1, AO: -1}
}

func modelTriangle(name string, origin float32, material modeldata.Material) modeldata.Part {
	return modeldata.Part{
		Name: name, Material: material, Indices: modelIndices(0, 1, 2),
		Vertices: modelVertices([]Vec3{{origin, 0, 0}, {origin + 1, 0, 0}, {origin, 1, 0}}, []Vec2{{0, 0}, {1, 0}, {0, 1}}),
	}
}

func modelVertices(positions []Vec3, coordinates []Vec2) []byte {
	var data []byte

	for index, position := range positions {
		for _, value := range []float32{position[0], position[1], position[2], 0, 0, 1, coordinates[index][0], coordinates[index][1]} {
			data = binary.LittleEndian.AppendUint32(data, math.Float32bits(value))
		}
	}

	return data
}

func modelIndices(indices ...uint32) []byte {
	var data []byte
	for _, index := range indices {
		data = binary.LittleEndian.AppendUint32(data, index)
	}

	return data
}

func modelUVBytes(coordinates []Vec2) []byte {
	var data []byte

	for _, coordinate := range coordinates {
		for _, value := range coordinate {
			data = binary.LittleEndian.AppendUint32(data, math.Float32bits(value))
		}
	}

	return data
}

func encodeModelFixture(t *testing.T, document modeldata.Document) []byte {
	t.Helper()

	data, err := modeldata.Encode(document)
	if err != nil {
		t.Fatal(err)
	}

	return data
}

func modelTexture(t *testing.T, usage string, pixel [4]byte) []byte {
	t.Helper()

	data, err := assetbuild.EncodeTexture(assetbuild.Image{
		Name: "fixture", Source: "fixture.png", Usage: usage, Width: 1, Height: 1, Levels: [][]byte{pixel[:]},
	})
	if err != nil {
		t.Fatal(err)
	}

	return data
}

func modelReceivers(t *testing.T, data []byte, coordinates [][]byte) []Receiver {
	t.Helper()

	receivers, err := ReceiversFromModel(data, coordinates, 16, 16)
	if err != nil {
		t.Fatal(err)
	}

	if len(receivers) == 0 {
		t.Fatal("fixture lost all receiver support")
	}

	return receivers
}

func assertReceiverMaterial(t *testing.T, receivers []Receiver, expected uint32) {
	t.Helper()

	for _, receiver := range receivers {
		if len(receiver.Patches) == 0 {
			t.Fatal("fixture lost receiver patches")
		}

		for _, patch := range receiver.Patches {
			if patch.Material != expected {
				t.Fatal("receiver bound to another chunk's batch material", patch.Material, expected)
			}
		}
	}
}

func TestModelBakeGeometryAdmissionRejectsInvalidIndices(t *testing.T) {
	t.Parallel()

	part := modeldata.Part{
		Name:     "bad",
		Material: modelMaterial(),
		Vertices: modelVertices([]Vec3{{0, 0, 0}, {1, 0, 0}, {0, 1, 0}}, []Vec2{{0, 0}, {1, 0}, {0, 1}}),
		Indices:  modelIndices(0, 1, 2),
	}
	data := encodeModelFixture(t, modeldata.Document{Source: "bad-indices", Parts: []modeldata.Part{part}})
	// Corrupt the admitted container after encoding: Decode defers mesh admission.
	binary.LittleEndian.PutUint32(data[len(data)-4:], math.MaxUint32)

	if _, err := SceneFromModel(data, "world", 1); err == nil {
		t.Fatal("visibility conversion admitted an out-of-range vertex")
	}

	if _, err := ReceiversFromModel(data, [][]byte{modelUVBytes([]Vec2{{0, 0}, {1, 0}, {0, 1}})}, 16, 16); err == nil {
		t.Fatal("receiver conversion admitted an out-of-range vertex")
	}
}
