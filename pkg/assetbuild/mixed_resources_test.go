//go:build !js

package assetbuild

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestImageStagesPreserveModelResources(t *testing.T) {
	t.Parallel()
	project := t.TempDir()
	ssbumpTestPNG(t, project, "assets/tile-normal.png", [4]byte{128, 128, 255, 255})

	model := fixturePlugin(func(plan *Plan) error {
		refs, err := plan.Add(Action{
			ID: "model", Processor: "model", Revision: "1",
			Outputs: []Slot{{Name: "data", Schema: "karty.static-model@1"}, {Name: "files", Schema: "files@1"}},
			Run: func(*Context) (Result, error) {
				files, encodeErr := json.Marshal(Bundle{"tree.kmodel": []byte("prepared model")})

				return Result{"data": []byte("model input"), "files": files}, encodeErr
			},
		})
		if err != nil {
			return err
		}

		plan.Resources = append(plan.Resources, Resource{ID: "tree", Ref: refs[0]})
		plan.Publish(refs[1])

		return nil
	})
	assertModel := fixturePlugin(func(plan *Plan) error {
		for _, resource := range plan.Resources {
			if resource.ID == "tree" && resource.Ref == (Ref{Action: "model", Slot: "data"}) {
				return nil
			}
		}

		t.Fatal("image stages removed or changed the model resource")

		return nil
	})
	config := Config{
		Input: []string{"assets/**"}, Output: "generated/assets",
		Plugins: []Plugin{
			model, Images(), Resize(ResizeOptions{Width: 2}),
			Materialize(MaterializeOptions{Include: []string{"tree"}, Executable: "missing-materialize"}),
			SSBump(SSBumpOptions{Rays: 8, Distance: 2}),
			Atlas(AtlasOptions{Name: "Tiles", Include: []string{"*"}, MaxSize: 32, Padding: 1, Clamp: true}),
			Mipmaps(), KTX2(), GoCatalog(), assertModel,
		},
	}

	options := Options{Project: project, Identity: "mixed-resource-fixture"}
	if _, err := Build(t.Context(), config, options); err != nil {
		t.Fatal(err)
	}

	data, err := fixtureRead(project, "generated/assets/tree.kmodel")
	if err != nil || !bytes.Equal(data, []byte("prepared model")) {
		t.Fatalf("model publication: %q %v", data, err)
	}

	report, err := Build(t.Context(), config, options)
	if err != nil || report.Executed != 0 || report.Reused == 0 {
		t.Fatalf("warm mixed pipeline: %+v %v", report, err)
	}
}
