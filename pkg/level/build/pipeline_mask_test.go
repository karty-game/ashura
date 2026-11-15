//go:build !js

package levelbuild

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/karty-game/karty-webgpu/internal/leveldata"
	"github.com/karty-game/karty-webgpu/internal/modeldata"
	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
	modelbuild "github.com/karty-game/karty-webgpu/pkg/model/build"
	"github.com/karty-game/karty-webgpu/pkg/scene/resource"
)

func TestStaticMixedMaskModelFailsBeforeDiscardingCoverage(t *testing.T) {
	t.Parallel()

	project := writeFixture(t)

	doc, err := modeldata.Decode(readLevelFixture(t, project, "floor.kmodel"))
	if err != nil {
		t.Fatal(err)
	}

	masked := doc.Parts[0]
	masked.Name = "masked-chain"
	masked.Material.AlphaMask = true
	masked.Material.AlphaCutoff = .375
	doc.Parts = append(doc.Parts, masked)

	data, err := modeldata.Encode(doc)
	if err != nil {
		t.Fatal(err)
	}

	if err = os.WriteFile(filepath.Join(project, "floor.kmodel"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	config := assetbuild.Config{Output: "generated",
		Pipelines: []assetbuild.Pipeline{{Name: "fixture",
			Input: []string{"*.scene.yaml", "*.kmodel"},
			Import: []assetbuild.Plugin{
				fixtureModels{},
				ImportScenes(ImportOptions{Input: []string{"test.scene.yaml"}}),
				StaticChunks(ChunkOptions{Size: [3]float32{1, 1, 1}}),
			},
		}},
	}

	_, err = assetbuild.Build(t.Context(), config, assetbuild.Options{Project: project, Identity: "static-mask-guard@1"})
	if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "masked-chain") ||
		!strings.Contains(err.Error(), "isDynamic: true") {
		t.Fatal("mixed static MASK coverage was accepted or lacked the independent-model remedy", err)
	}
}

type dynamicMaskFixtureModel struct{}

func (dynamicMaskFixtureModel) Name() string { return "dynamic-mask-model" }
func (dynamicMaskFixtureModel) Plan(plan *assetbuild.Plan) error {
	refs, err := plan.Add(assetbuild.Action{
		ID: "fixture/cutout-model", Processor: "fixture-model", Revision: "1",
		Outputs: []assetbuild.Slot{{Name: "model", Schema: modelbuild.PreparedSchema}},
		Run: func(ctx *assetbuild.Context) (assetbuild.Result, error) {
			data, err := ctx.ReadFile("cutouts.kmodel")

			return assetbuild.Result{"model": data}, err
		},
	})
	if err == nil {
		plan.Resources = append(plan.Resources, assetbuild.Resource{ID: "cutouts.glb", Ref: refs[0]})
	}

	return err
}

func addDynamicMaskFixture(t *testing.T, project string, config *assetbuild.Config) string {
	t.Helper()

	doc, err := modeldata.Decode(readLevelFixture(t, project, "floor.kmodel"))
	if err != nil {
		t.Fatal(err)
	}

	pixels := []byte{200, 180, 160, 0, 200, 180, 160, 64, 200, 180, 160, 128, 200, 180, 160, 255}

	albedo, err := assetbuild.EncodeTexture(assetbuild.Image{
		Name: "Cutout", Source: "cutout.png", Usage: assetbuild.UsageAlbedo,
		Width: 2, Height: 2, Levels: [][]byte{pixels},
	})
	if err != nil {
		t.Fatal(err)
	}

	doc.Source = "cutouts.glb"
	doc.Textures[0].Data = albedo
	doc.Parts[0].Name = "masked-chain"
	doc.Parts[0].Material.AlphaMask = true
	doc.Parts[0].Material.AlphaCutoff = .375

	data, err := modeldata.Encode(doc)
	if err != nil {
		t.Fatal(err)
	}

	if err = os.WriteFile(filepath.Join(project, "cutouts.kmodel"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	source := fixtureScene + `    - id: cutouts
      components:
        spatial.transform3d: {position: [0, 1, 0]}
        render3d.model: {asset: cutouts.glb, isDynamic: true}
`
	if err = os.WriteFile(filepath.Join(project, "test.scene.yaml"), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}

	config.Pipelines[0].Import = append([]assetbuild.Plugin{dynamicMaskFixtureModel{}}, config.Pipelines[0].Import...)

	return source
}

func verifyDynamicMaskFixture(t *testing.T, project string, doc leveldata.Document) {
	t.Helper()

	for _, object := range doc.Objects {
		if object.ID != "/cutouts" {
			continue
		}

		if !object.Dynamic || object.Model != "cutouts.glb" || object.Asset == nil || object.Transform[13] != 1 {
			t.Fatal("masked independent model lost its identity, payload or placement", object)
		}

		data, err := resource.Read(t.Context(), filepath.Join(project, "generated/public"), resource.Asset{
			ID: object.Model, Path: object.Asset.Path, Size: object.Asset.Size, SHA256: object.Asset.SHA256,
		})
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(data, readLevelFixture(t, project, "cutouts.kmodel")) {
			t.Fatal("dynamic MASK payload lost authored alpha coverage, cutoff or geometry")
		}

		return
	}

	t.Fatal("dynamic MASK model was omitted from publication")
}
