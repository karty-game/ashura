//go:build !js

package levelbuild

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/karty-game/karty-webgpu/internal/leveldata"
	"github.com/karty-game/karty-webgpu/internal/modeldata"
	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
	"github.com/karty-game/karty-webgpu/pkg/scene/resource"
)

func TestSeparateMaskedPartsPublishesCoverageOutsideOpaqueBake(t *testing.T) {
	t.Parallel()
	project := writeFixture(t)
	masked, textures := writeMixedMaskFixture(t, project)
	config := fixtureConfig(syntheticBake())

	config.Pipelines[0].Import[1] = Scenes(SceneOptions{Input: []string{"test.scene.yaml"}, SeparateMasked: true})
	if _, err := assetbuild.Build(
		t.Context(),
		config,
		assetbuild.Options{Project: project, Identity: "mixed-mask-separation@1"},
	); err != nil {
		t.Fatal(err)
	}

	manifest := findManifest(t, project)
	found := false

	for _, object := range manifest.Objects {
		if object.ID != "/floor/#masked" {
			continue
		}

		found = true

		identity := [16]float32{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}
		if !object.Dynamic || object.Asset == nil || object.Transform != identity {
			t.Fatal("independent mask lost its prepared identity pose", object)
		}

		assertPreparedMask(t, readPreparedModel(t, project, *object.Asset), masked, textures)
	}

	if !found {
		t.Fatal("mixed-model mask disappeared from publication")
	}

	for _, chunk := range manifest.Chunks {
		for _, page := range chunk.Pages {
			model := readPreparedModel(t, project, page.Geometry)
			for _, part := range model.Parts {
				if part.Material.AlphaMask || part.Name == masked.Name {
					t.Fatal("mask entered opaque chunk")
				}
			}
		}
	}
}

func writeMixedMaskFixture(t *testing.T, project string) (modeldata.Part, []modeldata.Texture) {
	t.Helper()

	document, err := modeldata.Decode(readLevelFixture(t, project, "floor.kmodel"))
	if err != nil {
		t.Fatal(err)
	}

	masked := document.Parts[0]
	masked.Name = "hanging-mask"
	masked.Material.AlphaMask = true
	masked.Material.AlphaCutoff = .375
	// This valid Standard normal texture is deliberately ineligible for the
	// square Source relief converter. It must stay outside the opaque bake.
	normal, err := assetbuild.EncodeTexture(
		assetbuild.Image{
			Name:   "MaskedNormal",
			Source: "normal.png",
			Usage:  assetbuild.UsageNormal,
			Width:  2,
			Height: 1,
			Levels: [][]byte{{128, 128, 255, 255, 128, 128, 255, 255}},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	document.Textures = append(document.Textures, document.Textures[0], modeldata.Texture{Role: modeldata.Normal, Data: normal})
	masked.Material.Albedo = 1
	masked.Material.Normal = 2
	document.Parts = append(document.Parts, masked)

	data, err := modeldata.Encode(document)
	if err != nil {
		t.Fatal(err)
	}

	if err = os.WriteFile(filepath.Join(project, "floor.kmodel"), data, 0600); err != nil {
		t.Fatal(err)
	}

	source := strings.Replace(
		fixtureScene,
		"    - id: floor\n      components:\n",
		"    - id: floor\n      components:\n        spatial.transform3d: {position: [2, 3, 4], scale: [2, 1, 1]}\n",
		1,
	)
	if err = os.WriteFile(filepath.Join(project, "test.scene.yaml"), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}

	return masked, document.Textures[1:]
}

func readPreparedModel(t *testing.T, project string, ref leveldata.Ref) modeldata.Document {
	t.Helper()

	data, err := resource.Read(
		t.Context(),
		filepath.Join(project, "generated/public"),
		resource.Asset{ID: "fixture", Path: ref.Path, Size: ref.Size, SHA256: ref.SHA256},
	)
	if err != nil {
		t.Fatal(err)
	}

	model, err := modeldata.Decode(data)
	if err != nil {
		t.Fatal(err)
	}

	return model
}

func assertPreparedMask(t *testing.T, model modeldata.Document, masked modeldata.Part, textures []modeldata.Texture) {
	t.Helper()

	if len(model.Parts) != 1 || len(model.Textures) != 2 {
		t.Fatal("incorrect masked subset size")
	}

	part := model.Parts[0]
	expected := masked.Material

	expected.Albedo, expected.Normal = 0, 1
	if part.Material != expected || !bytes.Equal(part.Indices, masked.Indices) {
		t.Fatal("masked material or index bytes changed")
	}

	for index, image := range model.Textures {
		if image.Role != textures[index].Role || !bytes.Equal(image.Data, textures[index].Data) {
			t.Fatal("masked texture role or bytes changed", index)
		}
	}

	for offset := 0; offset < len(masked.Vertices); offset += 32 {
		for axis := range 3 {
			value := math.Float32frombits(binary.LittleEndian.Uint32(masked.Vertices[offset+axis*4:]))
			if axis == 0 {
				value *= 2
			}

			value += float32(axis + 2)

			actual := math.Float32frombits(binary.LittleEndian.Uint32(part.Vertices[offset+axis*4:]))
			if actual != value {
				t.Fatal("mask world placement changed", actual, value)
			}
		}

		if !bytes.Equal(part.Vertices[offset+24:offset+32], masked.Vertices[offset+24:offset+32]) {
			t.Fatal("mask UV bytes changed")
		}
	}
}
