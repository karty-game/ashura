//go:build !js

//nolint:errcheck,forcetypeassert // Type assertions access only the engine-owned fixture schema.
package modelbuild_test

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"

	"github.com/karty-game/karty-webgpu/internal/modelbuild"
	"github.com/karty-game/karty-webgpu/internal/modelbuild/testfixture"
	"github.com/karty-game/karty-webgpu/internal/modeldata"
	"github.com/karty-game/karty-webgpu/pkg/texture"
	"github.com/karty-game/karty-webgpu/pkg/texture/ktx2"
)

func TestHierarchyMirrorAndDeterministicPreparedBytes(t *testing.T) {
	t.Parallel()

	document, data := testfixture.Triangle()
	document["nodes"] = []any{
		map[string]any{"children": []int{1}, "translation": []float64{1, 0, 0}},
		map[string]any{"mesh": 0, "translation": []float64{0, 2, 0}, "scale": []float64{-2, 3, 1}},
	}
	input := testfixture.GLB(document, data)

	first, err := modelbuild.Compile(input, "hierarchy.glb")
	if err != nil {
		t.Fatal(err)
	}

	second, err := modelbuild.Compile(input, "hierarchy.glb")
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(first.Data, second.Data) {
		t.Fatal("prepared output is nondeterministic")
	}

	prepared, err := modeldata.Decode(first.Data)
	if err != nil {
		t.Fatal(err)
	}

	part := prepared.Parts[0]

	value := func(offset int) float32 {
		return math.Float32frombits(binary.LittleEndian.Uint32(part.Vertices[offset:]))
	}
	if value(0) != 2 || value(4) != .5 || value(20) != 1 {
		t.Fatalf("baked first vertex/normal = %v,%v,%v", value(0), value(4), value(20))
	}

	if binary.LittleEndian.Uint32(part.Indices[4:]) != 2 || binary.LittleEndian.Uint32(part.Indices[8:]) != 1 {
		t.Fatal("mirrored node did not preserve front-face winding")
	}

	if part.Material.DoubleSided {
		t.Fatal("glTF default single-sided state was lost")
	}

	if first.Parts != 1 || first.VertexBytes != 96 || first.IndexBytes != 12 {
		t.Fatalf("unexpected resource report: %#v", first)
	}
}

func TestEmbeddedTextureRolesAndFlatNormalsAreOffline(t *testing.T) {
	t.Parallel()

	document, data := testfixture.Triangle()
	data = testfixture.EmbedTexture(document, data)
	material := document["materials"].([]any)[0].(map[string]any)
	material["normalTexture"] = map[string]any{"index": 0, "scale": .25}
	material["occlusionTexture"] = map[string]any{"index": 0, "strength": 0}
	attributes := document["meshes"].([]any)[0].(map[string]any)["primitives"].([]any)[0].(map[string]any)["attributes"].(map[string]any)
	delete(attributes, "NORMAL")

	result, err := modelbuild.Compile(testfixture.GLB(document, data), "textures.glb")
	if err != nil {
		t.Fatal(err)
	}

	prepared, err := modeldata.Decode(result.Data)
	if err != nil {
		t.Fatal(err)
	}

	if len(prepared.Textures) != 3 {
		t.Fatalf("got %d role-specific textures", len(prepared.Textures))
	}

	for index, image := range prepared.Textures {
		decoded, decodeErr := ktx2.Decode(image.Data)
		if decodeErr != nil {
			t.Fatal(decodeErr)
		}

		space := texture.Linear
		if index == 0 {
			space = texture.SRGB
		}

		if decoded.ColorSpace != space || len(decoded.Levels) != 2 {
			t.Fatalf("texture %d missing offline color/mip preparation", index)
		}
	}

	part := prepared.Parts[0]
	if part.Material.NormalStrength != .25 || part.Material.AOStrength != 0 {
		t.Fatal("material strengths were not preserved")
	}

	if math.Float32frombits(binary.LittleEndian.Uint32(part.Vertices[20:])) != 1 {
		t.Fatal("missing normals were not baked into GPU-ready vertices")
	}
}

func TestMultiPrimitiveMaterialsAndFlattenedSample(t *testing.T) {
	t.Parallel()

	result, err := modelbuild.Compile(testfixture.Scene(), "scene.glb")
	if err != nil {
		t.Fatal(err)
	}

	document, err := modeldata.Decode(result.Data)
	if err != nil {
		t.Fatal(err)
	}

	if len(document.Parts) != 2 || len(document.Textures) != 1 {
		t.Fatalf("parts/textures = %d/%d", len(document.Parts), len(document.Textures))
	}

	if document.Parts[0].Material.Albedo != 0 || document.Parts[1].Material.Albedo != -1 || !document.Parts[1].Material.DoubleSided {
		t.Fatal("per-node material assignment was lost")
	}

	if bytes.Equal(document.Parts[0].Vertices, document.Parts[1].Vertices) {
		t.Fatal("distinct baked node transforms were lost")
	}
}

func TestOpaqueSourceAlphaIgnoredOffline(t *testing.T) {
	t.Parallel()

	document, data := testfixture.Triangle()
	document["materials"].([]any)[0].(map[string]any)["pbrMetallicRoughness"].(map[string]any)["baseColorFactor"] = []float64{1, .5, .25, 0}
	document["materials"].([]any)[0].(map[string]any)["alphaCutoff"] = .4

	result, err := modelbuild.Compile(testfixture.GLB(document, data), "opaque.glb")
	if err != nil {
		t.Fatal(err)
	}

	prepared, err := modeldata.Decode(result.Data)
	if err != nil {
		t.Fatal(err)
	}

	if prepared.Parts[0].Material.Color[3] != 1 {
		t.Fatal("OPAQUE retained coverage alpha")
	}
}
