//go:build !js

//nolint:errcheck,forcetypeassert // Type assertions access only the engine-owned fixture schema.
package modelbuild_test

import (
	"encoding/binary"
	"testing"

	"github.com/karty-game/ashura/internal/modelbuild"
	"github.com/karty-game/ashura/internal/modelbuild/testfixture"
)

func TestMalformedAndUnsupportedGLBFailExplicitly(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"animation", "skin", "camera", "extension", "morph", "tangent", "metallic-default", "missing-material", "blend", "emissive", "external-buffer", "external-image", "bad-index", "view-bounds", "stride", "accessor-count", "normalized", "sparse", "cycle", "multiple-parents", "bad-root", "singular", "bad-quaternion", "uv-set", "sampler"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			document, data := testfixture.Triangle()
			primitive := document["meshes"].([]any)[0].(map[string]any)["primitives"].([]any)[0].(map[string]any)
			material := document["materials"].([]any)[0].(map[string]any)

			switch name {
			case "animation":
				document["animations"] = []any{map[string]any{}}
			case "skin":
				document["skins"] = []any{map[string]any{}}
			case "camera":
				document["cameras"] = []any{map[string]any{}}
			case "extension":
				material["extensions"] = map[string]any{"KHR_materials_unlit": map[string]any{}}
			case "morph":
				primitive["targets"] = []any{map[string]any{"POSITION": 0}}
			case "tangent":
				primitive["attributes"].(map[string]any)["TANGENT"] = 0
			case "metallic-default":
				delete(material["pbrMetallicRoughness"].(map[string]any), "metallicFactor")
			case "missing-material":
				delete(primitive, "material")
			case "blend":
				material["alphaMode"] = "BLEND"
			case "emissive":
				material["emissiveFactor"] = []float64{1, 0, 0}
			case "external-buffer":
				document["buffers"].([]any)[0].(map[string]any)["uri"] = "../outside.bin"
			case "external-image":
				document["images"] = []any{map[string]any{"uri": "../../outside.png"}}
			case "bad-index":
				binary.LittleEndian.PutUint16(data[100:], 3)
			case "view-bounds":
				document["bufferViews"].([]any)[0].(map[string]any)["byteLength"] = 10000
			case "stride":
				document["bufferViews"].([]any)[0].(map[string]any)["byteStride"] = 5
			case "accessor-count":
				document["accessors"].([]any)[0].(map[string]any)["count"] = 10000
			case "normalized":
				document["accessors"].([]any)[0].(map[string]any)["normalized"] = true
			case "sparse":
				document["accessors"].([]any)[0].(map[string]any)["sparse"] = map[string]any{}
			case "cycle":
				document["nodes"].([]any)[0].(map[string]any)["children"] = []int{0}
			case "multiple-parents":
				document["nodes"] = []any{
					map[string]any{"children": []int{2}},
					map[string]any{"children": []int{2}},
					map[string]any{"mesh": 0},
				}
			case "bad-root":
				document["scenes"].([]any)[0].(map[string]any)["nodes"] = []int{9}
			case "singular":
				document["nodes"].([]any)[0].(map[string]any)["scale"] = []float64{0, 1, 1}
			case "bad-quaternion":
				document["nodes"].([]any)[0].(map[string]any)["rotation"] = []float64{0, 0, 0, 2}
			case "uv-set":
				data = testfixture.EmbedTexture(document, data)
				material["pbrMetallicRoughness"].(map[string]any)["baseColorTexture"] = map[string]any{"index": 0, "texCoord": 1}
			case "sampler":
				document["samplers"] = []any{map[string]any{"wrapS": 33071}}
			}

			if _, err := modelbuild.Compile(testfixture.GLB(document, data), name+".glb"); err == nil {
				t.Fatalf("accepted %s", name)
			}
		})
	}
}

func TestTruncatedAndCorruptGLBHeader(t *testing.T) {
	t.Parallel()

	document, data := testfixture.Triangle()

	valid := testfixture.GLB(document, data)
	for _, length := range []int{0, 4, 11, 12, 19, len(valid) - 1} {
		if _, err := modelbuild.Compile(valid[:length], "truncated.glb"); err == nil {
			t.Fatalf("accepted %d bytes", length)
		}
	}

	corrupt := append([]byte(nil), valid...)

	corrupt[0] = 'X'
	if _, err := modelbuild.Compile(corrupt, "magic.glb"); err == nil {
		t.Fatal("accepted wrong GLB magic")
	}
}

func TestReverseIndexedDeepHierarchyRejected(t *testing.T) {
	t.Parallel()

	document, data := testfixture.Triangle()

	nodes := []any{map[string]any{"mesh": 0}}
	for index := 1; index < 129; index++ {
		nodes = append(nodes, map[string]any{"children": []int{index - 1}})
	}

	document["nodes"] = nodes

	document["scenes"].([]any)[0].(map[string]any)["nodes"] = []int{128}
	if _, err := modelbuild.Compile(testfixture.GLB(document, data), "reverse-depth.glb"); err == nil {
		t.Fatal("reverse-index hierarchy bypassed depth bound")
	}
}

func TestAuthoredEmptyTransformArraysAreInvalid(t *testing.T) {
	t.Parallel()

	for _, field := range []string{"matrix", "translation", "rotation", "scale"} {
		document, data := testfixture.Triangle()

		document["nodes"].([]any)[0].(map[string]any)[field] = []float64{}
		if _, err := modelbuild.Compile(testfixture.GLB(document, data), "empty-"+field+".glb"); err == nil {
			t.Fatalf("accepted empty %s", field)
		}
	}
}
