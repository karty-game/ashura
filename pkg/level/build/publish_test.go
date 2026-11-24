//go:build !js

package levelbuild

import (
	"bytes"
	"errors"
	"os"
	"testing"

	"github.com/karty-game/ashura/internal/leveldata"
	"github.com/karty-game/ashura/pkg/assetbuild"
)

type emptyPublishedLevel struct{}

func (emptyPublishedLevel) Name() string { return "empty-level-fixture" }
func (emptyPublishedLevel) Plan(plan *assetbuild.Plan) error {
	refs, err := plan.Add(
		assetbuild.Action{
			ID:        "empty-level",
			Processor: "empty-level-fixture",
			Revision:  "1",
			Outputs:   []assetbuild.Slot{{Name: slot, Schema: bakedSchema}},
			Run: func(*assetbuild.Context) (assetbuild.Result, error) {
				return result(partitioned{Source: "test.scene.yaml", Size: [3]float32{32, 32, 32}, Density: 16})
			},
		},
	)
	if err != nil {
		return err
	}

	plan.Resources = append(plan.Resources, assetbuild.Resource{ID: "test.scene.yaml", Ref: refs[0]})

	return nil
}

func TestInvalidPublisherPackagePreservesCompleteGeneration(t *testing.T) {
	t.Parallel()
	project := t.TempDir()
	config := assetbuild.Config{Output: "generated",
		Pipelines: []assetbuild.Pipeline{{Name: "fixture",
			Import: []assetbuild.Plugin{emptyPublishedLevel{}, Publish(PublishOptions{})},
		}},
	}

	options := assetbuild.Options{Project: project, Identity: "publisher-admission"}
	if _, err := assetbuild.Build(t.Context(), config, options); err != nil {
		t.Fatal(err)
	}

	before, err := leveldata.Encode(findManifest(t, project))
	if err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"bad-name", "for", "_", "9invalid", "../escape"} {
		config.Pipelines[0].Import[1] = Publish(PublishOptions{Package: name})
		if _, err := assetbuild.Build(t.Context(), config, options); err == nil {
			t.Fatalf("invalid descriptor package %q accepted", name)
		}

		after, err := leveldata.Encode(findManifest(t, project))
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(before, after) {
			t.Fatalf("invalid package %q changed last valid generation", name)
		}
	}
}

func TestInvalidGeneratedLevelIdentifierIsRejected(t *testing.T) {
	t.Parallel()

	for _, source := range []string{"_.scene.yaml", "---.scene.yaml"} {
		if err := publishLevelManifest(
			&assetbuild.Plan{},
			assetbuild.Resource{ID: source},
			partitioned{Source: source},
			leveldata.Document{},
			PublishOptions{Package: "levels"},
		); err == nil {
			t.Fatalf("invalid generated basename %q accepted", source)
		}
	}
}

func TestPublisherPackageChangeInvalidatesDescriptorOnly(t *testing.T) {
	t.Parallel()
	project := t.TempDir()
	config := assetbuild.Config{Output: "generated",
		Pipelines: []assetbuild.Pipeline{{Name: "fixture",
			Import: []assetbuild.Plugin{emptyPublishedLevel{}, Publish(PublishOptions{})},
		}},
	}
	options := assetbuild.Options{Project: project, Identity: "publisher-package-cache"}

	if _, err := assetbuild.Build(t.Context(), config, options); err != nil {
		t.Fatal(err)
	}

	before, err := leveldata.Encode(findManifest(t, project))
	if err != nil {
		t.Fatal(err)
	}

	config.Pipelines[0].Import[1] = Publish(PublishOptions{Package: "customlevels"})

	changed, err := assetbuild.Build(t.Context(), config, options)
	if err != nil {
		t.Fatal(err)
	}

	if changed.Executed != 1 || changed.Removed != 1 {
		t.Fatalf("package change did not replace only the descriptor: %+v", changed)
	}

	code := readLevelFixture(t, project, "generated/customlevels/Test.go")
	if !bytes.Contains(code, []byte("package customlevels")) {
		t.Fatal("generated descriptor retained the cached package")
	}

	root, err := os.OpenRoot(project)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()

	if _, err := root.Stat("generated/levels/Test.go"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("old descriptor retained: %v", err)
	}

	after, err := leveldata.Encode(findManifest(t, project))
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("descriptor package change modified the level: %v", err)
	}

	warm, err := assetbuild.Build(t.Context(), config, options)
	if err != nil || warm.Executed != 0 {
		t.Fatalf("new descriptor package did not cache: %+v: %v", warm, err)
	}
}

func publicationMetadata(scale float32, swap bool) assetbuild.Plugin {
	return stage{name: "fixture-metadata", schema: bakedSchema, run: func(plan *assetbuild.Plan, resource assetbuild.Resource) error {
		refs, err := plan.Add(assetbuild.Action{
			ID: "fixture/metadata", Processor: "fixture-metadata", Revision: "1", Inputs: []assetbuild.Ref{resource.Ref},
			Options: struct {
				Scale float32
				Swap  bool
			}{scale, swap}, Outputs: []assetbuild.Slot{{Name: slot, Schema: bakedSchema}},
			Run: func(ctx *assetbuild.Context) (assetbuild.Result, error) {
				var record partitioned
				if err := readRecord(ctx, resource.Ref, &record); err != nil {
					return nil, err
				}

				for ci := range record.Chunks {
					for pi := range record.Chunks[ci].Pages {
						page := &record.Chunks[ci].Pages[pi]
						pixels := bytes.Repeat([]byte{64, 32, 16, 125}, int(page.Width*page.Height))

						data, err := assetbuild.EncodeTexture(
							assetbuild.Image{
								Name:   "second",
								Source: "second.png",
								Usage:  assetbuild.UsageLightmap,
								Width:  int(page.Width),
								Height: int(page.Height),
								Levels: [][]byte{pixels},
							},
						)
						if err != nil {
							return nil, err
						}

						page.Maps[1], err = ctx.StoreArtifact(data)
						if err != nil {
							return nil, err
						}

						if swap {
							page.Maps[0], page.Maps[1] = page.Maps[1], page.Maps[0]
						}

						page.Scale = scale
					}
				}

				return result(record)
			},
		})
		if err != nil {
			return err
		}

		replaceResource(plan, resource.ID, refs[0])

		return nil
	}}
}

func TestPublicationCacheTracksOrderedMapBindingsAndScale(t *testing.T) {
	t.Parallel()

	project := writeFixture(t)
	config := fixtureConfig(syntheticBake())
	config.Pipelines[0].Process = append(config.Pipelines[0].Process, publicationMetadata(1, false))
	metadataIndex := len(config.Pipelines[0].Process) - 1

	options := assetbuild.Options{Project: project, Identity: "publication-metadata-cache"}
	if _, err := assetbuild.Build(t.Context(), config, options); err != nil {
		t.Fatal(err)
	}

	first := findManifest(t, project).Chunks[0].Pages[0]
	config.Pipelines[0].Process[metadataIndex] = publicationMetadata(2, true)

	changed, err := assetbuild.Build(t.Context(), config, options)
	if err != nil {
		t.Fatal(err)
	}

	second := findManifest(t, project).Chunks[0].Pages[0]
	if first.Maps[0] == first.Maps[1] || second.Maps[0] != first.Maps[1] || second.Maps[1] != first.Maps[0] || second.Scale != 2 {
		t.Fatalf("stale map bindings or scale: first=%+v second=%+v", first, second)
	}

	if first.Geometry != second.Geometry || first.UV != second.UV {
		t.Fatal("lighting metadata changed prepared geometry")
	}

	for _, action := range changed.Actions {
		if (action.Processor == "level-chunks" || action.Processor == "level-uv") && action.Status != "HIT" {
			t.Fatalf("lighting metadata invalidated geometry: %+v", action)
		}
	}
}
