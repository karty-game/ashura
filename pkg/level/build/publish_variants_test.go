//go:build !js

package levelbuild

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/karty-game/karty-webgpu/internal/leveldata"
	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
)

func TestVariantDescriptorsHaveIndependentOwnership(t *testing.T) {
	t.Parallel()
	project := t.TempDir()
	config := assetbuild.Config{Output: "generated",
		Pipelines: []assetbuild.Pipeline{{Name: "fixture",
			Import: []assetbuild.Plugin{
				emptyPublishedLevel{},
				PublishVariants(PublishOptions{Variant: "BC"}, PublishOptions{Variant: "Mobile"}),
			},
		}},
	}

	options := assetbuild.Options{Project: project, Identity: "variant-ownership"}
	if _, err := assetbuild.Build(t.Context(), config, options); err != nil {
		t.Fatal(err)
	}

	for _, variant := range []string{"BC", "Mobile"} {
		descriptors, err := filepath.Glob(filepath.Join(project, "generated", "levels", "Test"+variant+".go"))
		if err != nil || len(descriptors) != 1 {
			t.Fatal(descriptors, err)
		}

		manifests, err := filepath.Glob(filepath.Join(project, "generated", "public", "levels", "profiles", variant, "*.json"))
		if err != nil || len(manifests) != 1 {
			t.Fatal(manifests, err)
		}
	}
	// The same manifest content may own two named profiles without colliding,
	// including an unchanged build whose producing actions are cache hits.
	if _, err := assetbuild.Build(t.Context(), config, options); err != nil {
		t.Fatal(err)
	}
}

type sharedPayloadFixture struct{}

func (sharedPayloadFixture) Name() string { return "shared-payload-fixture" }
func (sharedPayloadFixture) Plan(plan *assetbuild.Plan) error {
	refs, err := plan.Add(
		assetbuild.Action{
			ID:        "shared-seed",
			Processor: "fixture",
			Revision:  "1",
			Outputs:   []assetbuild.Slot{{Name: slot, Schema: filesSchema}},
			Run: func(ctx *assetbuild.Context) (assetbuild.Result, error) {
				file, err := storePublished(ctx, "fixture", ".bin", []byte("shared"))
				if err != nil {
					return nil, err
				}

				return result(file)
			},
		},
	)
	if err != nil {
		return err
	}

	return plan.Expand(
		assetbuild.Expansion{ID: "shared-files", Inputs: refs, Run: func(ctx *assetbuild.Context, child *assetbuild.Plan) error {
			var file fileRecord
			if err := readRecord(ctx, refs[0], &file); err != nil {
				return err
			}

			files := map[string]fileRecord{file.Reference.Path: file}

			shared := map[string]fileRecord{}
			if err := publishSharedFiles(child, "BC", files, shared); err != nil {
				return err
			}

			return publishSharedFiles(child, "Mobile", files, shared)
		}},
	)
}

func TestVariantsRetainOneSharedPayloadOwner(t *testing.T) {
	t.Parallel()

	project := t.TempDir()
	if _, err := assetbuild.Build(
		t.Context(),
		assetbuild.Config{Output: "generated",
			Pipelines: []assetbuild.Pipeline{{Name: "fixture",
				Import: []assetbuild.Plugin{sharedPayloadFixture{}},
			}},
		},
		assetbuild.Options{Project: project, Identity: "shared-profile-payload"},
	); err != nil {
		t.Fatal(err)
	}

	names, err := filepath.Glob(filepath.Join(project, "generated", "public", "levels", "fixture", "*.bin"))
	if err != nil || len(names) != 1 {
		t.Fatal(names, err)
	}

	data, err := os.ReadFile(names[0])
	if err != nil || string(data) != "shared" {
		t.Fatal(string(data), err)
	}
}

func TestLargeAtlasDoesNotDowngradeEncodingVersion(t *testing.T) {
	t.Parallel()

	for _, schema := range []string{leveldata.Schema, leveldata.Schema2, leveldata.Schema3, leveldata.Schema4, leveldata.Schema5} {
		t.Run(schema, func(t *testing.T) {
			t.Parallel()

			expected := schema
			if schema == leveldata.Schema {
				expected = leveldata.Schema2
			}

			if actual := pagePublicationSchema(schema, leveldata.Page{Width: 2048, Height: 2048}); actual != expected {
				t.Fatal(actual, expected)
			}
		})
	}
}
