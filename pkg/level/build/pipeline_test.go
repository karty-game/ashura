//go:build !js

package levelbuild

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/karty-game/karty-webgpu/internal/leveldata"
	"github.com/karty-game/karty-webgpu/internal/modeldata"
	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
	"github.com/karty-game/karty-webgpu/pkg/lightmap"
	modelbuild "github.com/karty-game/karty-webgpu/pkg/model/build"
	"github.com/karty-game/karty-webgpu/pkg/scene/resource"
)

type fixtureModels struct{}

func (fixtureModels) Name() string { return "fixture-models" }
func (fixtureModels) Plan(plan *assetbuild.Plan) error {
	refs, err := plan.Add(
		assetbuild.Action{
			ID:        "fixture/model",
			Processor: "fixture-model",
			Revision:  "1",
			Outputs:   []assetbuild.Slot{{Name: "model", Schema: modelbuild.PreparedSchema}},
			Run: func(ctx *assetbuild.Context) (assetbuild.Result, error) {
				data, err := ctx.ReadFile("floor.kmodel")

				return assetbuild.Result{"model": data}, err
			},
		},
	)
	if err == nil {
		plan.Resources = append(plan.Resources, assetbuild.Resource{ID: "floor.glb", Ref: refs[0]})
	}

	return err
}

func fixtureConfig(hardware assetbuild.Plugin) assetbuild.Config {
	return assetbuild.Config{Output: "generated",
		Pipelines: []assetbuild.Pipeline{{Name: "fixture",
			Input: []string{"*.scene.yaml", "*.kmodel"},
			Import: []assetbuild.Plugin{
				fixtureModels{},
				Scenes(SceneOptions{Input: []string{"test.scene.yaml"}}),
			},
			Process: []assetbuild.Plugin{
				Materials(MaterialOptions{}),
				StaticChunks(ChunkOptions{Size: [3]float32{1, 1, 1}}),
				LightmapUVs(UVOptions{TexelsPerUnit: 4, MaxPageSize: 32, Padding: 2, TransportSamples: 16}),
				hardware,
			},
			Output: []assetbuild.Plugin{Publish(PublishOptions{})},
		}},
	}
}

// syntheticBake isolates graph/publication behavior; real shader correctness is
// exercised by the separate opt-in hardware test using the same input fixture.
func syntheticBake() assetbuild.Plugin {
	return stage{name: "fixture-bake", schema: uvSchema, run: func(plan *assetbuild.Plan, resource assetbuild.Resource) error {
		refs, err := plan.Add(
			assetbuild.Action{
				ID:        "fixture/bake/" + resource.ID,
				Processor: "fixture-bake",
				Revision:  "1",
				Inputs:    []assetbuild.Ref{resource.Ref},
				Outputs:   []assetbuild.Slot{{Name: slot, Schema: bakedSchema}},
				Run: func(ctx *assetbuild.Context) (assetbuild.Result, error) {
					var record partitioned
					if err := readRecord(ctx, resource.Ref, &record); err != nil {
						return nil, err
					}

					for ci := range record.Chunks {
						for pi := range record.Chunks[ci].Pages {
							page := &record.Chunks[ci].Pages[pi]
							pixels := bytes.Repeat([]byte{128, 96, 64, 125}, int(page.Width*page.Height))

							encoded, err := assetbuild.EncodeTexture(
								assetbuild.Image{
									Name:   "RNM",
									Source: "rnm.png",
									Usage:  assetbuild.UsageLightmap,
									Width:  int(page.Width),
									Height: int(page.Height),
									Levels: [][]byte{pixels},
								},
							)
							if err != nil {
								return nil, err
							}

							artifact, err := ctx.StoreArtifact(encoded)
							if err != nil {
								return nil, err
							}

							page.Maps = [3]assetbuild.Artifact{artifact, artifact, artifact}
							page.Scale = 1
							page.Encoding = "rgbe8"
						}
					}

					return result(record)
				},
			},
		)
		if err == nil {
			replaceResource(plan, resource.ID, refs[0])
		}

		return err
	}}
}

func writeFixture(t *testing.T) string {
	t.Helper()
	project := t.TempDir()

	var vertices []byte

	for _, v := range [][8]float32{{-1, 0, 0, 0, 1, 0, 0, 0}, {3, 0, 0, 0, 1, 0, 4, 0}, {3, 0, 1, 0, 1, 0, 4, 1}, {-1, 0, 1, 0, 1, 0, 0, 1}} {
		for _, f := range v {
			vertices = binary.LittleEndian.AppendUint32(vertices, math.Float32bits(f))
		}
	}

	var indices []byte
	for _, i := range []uint32{0, 2, 1, 0, 3, 2} {
		indices = binary.LittleEndian.AppendUint32(indices, i)
	}

	albedo, err := assetbuild.EncodeTexture(
		assetbuild.Image{
			Name:   "Albedo",
			Source: "floor.png",
			Usage:  assetbuild.UsageAlbedo,
			Width:  2,
			Height: 2,
			Levels: [][]byte{bytes.Repeat([]byte{200, 180, 160, 255}, 4)},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	model, err := modeldata.Encode(
		modeldata.Document{
			Source:   "floor.glb",
			Textures: []modeldata.Texture{{Role: modeldata.Albedo, Data: albedo}},
			Parts: []modeldata.Part{
				{
					Name:     "floor",
					Vertices: vertices,
					Indices:  indices,
					Material: modeldata.Material{
						Color:       [4]float32{1, 1, 1, 1},
						Albedo:      0,
						Normal:      -1,
						AO:          -1,
						Roughness:   1,
						DoubleSided: true,
					},
				},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	if err = os.WriteFile(filepath.Join(project, "floor.kmodel"), model, 0o600); err != nil {
		t.Fatal(err)
	}

	if err = os.WriteFile(filepath.Join(project, "test.scene.yaml"), []byte(fixtureScene), 0o600); err != nil {
		t.Fatal(err)
	}

	return project
}

const fixtureScene = `version: 1
root:
  id: level
  children:
    - id: floor
      components:
        render3d.model: {asset: floor.glb}
    - id: moving
      components:
        spatial.transform3d: {position: [12, 0, 0]}
        render3d.model: {asset: floor.glb, isDynamic: true}
    - id: power
      save_id: light-power
      components:
        logic.bool: {initial: true}
    - id: lamp
      components:
        spatial.transform3d: {position: [1, 2, 0.5]}
        render3d.point: {enabled: /power, bake: static, intensity: 2, range: 8}
`

func findManifest(t *testing.T, project string) leveldata.Document {
	t.Helper()

	names, err := filepath.Glob(filepath.Join(project, "generated/public/levels/*.json"))
	if err != nil || len(names) != 1 {
		t.Fatalf("manifest files=%v err=%v", names, err)
	}

	data, err := os.ReadFile(names[0])
	if err != nil {
		t.Fatal(err)
	}

	doc, err := leveldata.Decode(data)
	if err != nil {
		t.Fatal(err)
	}

	return doc
}

func TestLevelPipelineExternalChunksCacheAndLogicalIdentity(t *testing.T) {
	t.Parallel()

	project := writeFixture(t)
	config := fixtureConfig(syntheticBake())
	sceneSource := addDynamicMaskFixture(t, project, &config)
	options := assetbuild.Options{Project: project, Identity: "level-fixture@1"}

	cold, err := assetbuild.Build(t.Context(), config, options)
	if err != nil {
		t.Fatal(err)
	}

	doc := findManifest(t, project)
	if len(doc.Chunks) != 4 {
		t.Fatalf("expected4 staticcells, got%d", len(doc.Chunks))
	}

	verifyExternalChunks(t, project, doc)

	verifyLogicalObjects(t, project, doc)
	verifyDynamicMaskFixture(t, project, doc)

	files, _ := filepath.Glob(filepath.Join(project, "generated/public/levels/development/*"))
	if len(files) > 0 {
		t.Fatal("production shippedbakeinputs")
	}

	code := readLevelFixture(t, project, "generated/scenes/Test_gen.go")
	if !strings.Contains(string(code), "NewBakedModel(") || !strings.Contains(string(code), "NewModel(") {
		t.Fatal("logicalfactory lostpresentation split")
	}

	schema := readLevelFixture(t, project, "generated/scenes/scene.schema.json")
	if !json.Valid(schema) || !bytes.Contains(schema, []byte(`"isDynamic"`)) || !bytes.Contains(schema, []byte(`"bake"`)) {
		t.Fatal("level adapter did not publish its exact authoring schema")
	}

	warm, err := assetbuild.Build(t.Context(), config, options)
	if err != nil {
		t.Fatal(err)
	}

	if warm.Executed != 0 || warm.Reused != cold.Executed {
		t.Fatalf("warm cachecold=%+v warm=%+v", cold, warm)
	}

	scene := strings.Replace(sceneSource, "intensity: 2", "intensity: 3", 1)
	if err = os.WriteFile(filepath.Join(project, "test.scene.yaml"), []byte(scene), 0o600); err != nil {
		t.Fatal(err)
	}

	changed, err := assetbuild.Build(t.Context(), config, options)
	if err != nil {
		t.Fatal(err)
	}

	for _, action := range changed.Actions {
		if (action.Processor == "level-chunks" || action.Processor == "level-uv") && action.Status != "HIT" {
			t.Fatalf("lightedit rebuilt geometryUV: %+v", action)
		}
	}
	// A failed generation must leave the last complete public level untouched.
	before, err := json.Marshal(findManifest(t, project))
	if err != nil {
		t.Fatal(err)
	}

	if err = os.WriteFile(filepath.Join(project, "test.scene.yaml"), []byte("version: 99\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err = assetbuild.Build(t.Context(), config, options); err == nil {
		t.Fatal("acceptedmalformedlevel")
	}

	after, err := json.Marshal(findManifest(t, project))
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(before, after) {
		t.Fatal("failedbuildchangedpublishedlevel")
	}

	t.Logf("cold=%d actions warm=%d reused chunks=%d", cold.Executed, warm.Reused, len(doc.Chunks))
}

func TestLevelPipelineRejectsMissingStage(t *testing.T) {
	t.Parallel()
	project := t.TempDir()

	_, err := assetbuild.Build(
		t.Context(),
		assetbuild.Config{Output: "generated",
			Pipelines: []assetbuild.Pipeline{{Name: "fixture",
				Input:  []string{"*.scene.yaml"},
				Import: []assetbuild.Plugin{StaticChunks(ChunkOptions{})},
			}},
		},
		assetbuild.Options{Project: project, Identity: "fixture"},
	)
	if err == nil {
		t.Fatal("acceptedmissingimportstage")
	}

	t.Log(fmt.Sprint(err))
}

func verifyExternalChunks(t *testing.T, project string, doc leveldata.Document) {
	t.Helper()

	albedos := map[string]bool{}

	for _, chunk := range doc.Chunks {
		if len(chunk.Sources) != 1 || chunk.Sources[0] != "/floor" {
			t.Fatalf("dynamic/identity leaked into merged chunk: %+v", chunk)
		}

		for _, page := range chunk.Pages {
			if page.BakeInputs != nil {
				t.Fatal("production page exposes developer bake inputs")
			}

			geometry := readLevelFixture(t, project, filepath.Join("generated/public", page.Geometry.Path))

			model, err := modeldata.Decode(geometry)
			if err != nil {
				t.Fatal(err)
			}

			if len(model.Textures) != 0 {
				t.Fatal("embedded sharedtextures")
			}

			uv := readLevelFixture(t, project, filepath.Join("generated/public", page.UV.Path))

			layout, err := leveldata.DecodeUV(uv)
			if err != nil || layout.PartCount() != len(model.Parts) {
				t.Fatal("invalidlightmapbinding", err)
			}

			for _, part := range page.Materials {
				if part.Albedo == nil {
					t.Fatal("lostalbedo")
				}

				albedos[part.Albedo.SHA256] = true
			}
		}
	}

	if len(albedos) != 1 {
		t.Fatalf("shared texture duplicated: %v", albedos)
	}
}

func readLevelFixture(t *testing.T, project, relative string) []byte {
	t.Helper()

	root, err := os.OpenRoot(project)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = root.Close() }()

	data, err := root.ReadFile(relative)
	if err != nil {
		t.Fatal(err)
	}

	return data
}

func TestLevelDevelopmentPublishesDiscoverableBakeInputs(t *testing.T) {
	t.Parallel()

	project := writeFixture(t)
	config := fixtureConfig(syntheticBake())

	config.Pipelines[0].Output[0] = Publish(PublishOptions{Development: true})
	if _, err := assetbuild.Build(t.Context(), config, assetbuild.Options{Project: project, Identity: "developer-level"}); err != nil {
		t.Fatal(err)
	}

	document := findManifest(t, project)
	for _, chunk := range document.Chunks {
		for _, page := range chunk.Pages {
			if page.BakeInputs == nil {
				t.Fatal("developer page does not identify its prepared bake inputs")
			}

			data := readLevelFixture(t, project, filepath.Join("generated/public", page.BakeInputs.Path))

			asset, err := lightmap.Decode(data)
			if err != nil {
				t.Fatal(err)
			}

			width, height := asset.Size()
			if width != page.Width || height != page.Height || asset.PartCount() != len(page.Materials) || !asset.Document().Transport() {
				t.Fatal("developer bake inputs disagree with page dimensions/materials or lack prepared transport")
			}
		}
	}
}

func verifyLogicalObjects(t *testing.T, project string, doc leveldata.Document) {
	t.Helper()

	var dynamic bool

	for _, object := range doc.Objects {
		if object.ID == "/moving" {
			dynamic = object.Dynamic && object.Asset != nil
			if !dynamic || object.Model != "floor.glb" || object.Transform[12] != 12 {
				t.Fatal("independent model lost its source identity or placement", object)
			}

			payload, err := resource.Read(t.Context(), filepath.Join(project, "generated/public"), resource.Asset{
				ID: object.Model, Path: object.Asset.Path, Size: object.Asset.Size, SHA256: object.Asset.SHA256,
			})
			if err != nil {
				t.Fatal("independent model payload failed its manifest ownership", err)
			}

			original := readLevelFixture(t, project, "floor.kmodel")
			if !bytes.Equal(payload, original) {
				t.Fatal("independent model geometry, materials or embedded textures changed during publication")
			}
		}

		if object.ID == "/floor" && (object.Dynamic || object.Asset != nil) {
			t.Fatal("static placeholder acquired a second independent model payload")
		}

		if object.ID == "/power" && object.SaveID != "light-power" {
			t.Fatal("lostsaveidentity")
		}
	}

	if !dynamic {
		t.Fatal("lostdynamicplacement")
	}
}
