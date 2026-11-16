//go:build !js

package meshbuild_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/karty-game/karty-webgpu/internal/modelbuild/testfixture"
	"github.com/karty-game/karty-webgpu/internal/modeldata"
	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
	meshbuild "github.com/karty-game/karty-webgpu/pkg/mesh/build"
	modelbuild "github.com/karty-game/karty-webgpu/pkg/model/build"
)

var errFixtureModelCount = errors.New("expected one final prepared model")

func TestMeshPipelinePublishesFinalModelAndReusesVerifiedActions(t *testing.T) {
	t.Parallel()

	project := t.TempDir()
	if err := os.Mkdir(filepath.Join(project, "assets"), 0750); err != nil {
		t.Fatal(err)
	}

	source := filepath.Join(project, "assets/scene.glb")
	if err := os.WriteFile(source, testfixture.Scene(), 0600); err != nil {
		t.Fatal(err)
	}

	config := assetbuild.Config{Output: "generated", Pipelines: []assetbuild.Pipeline{{
		Name: "models", Input: []string{"assets/**"}, Import: []assetbuild.Plugin{modelbuild.PreparedGLB()},
		Process: []assetbuild.Plugin{meshbuild.Cleanup(), meshbuild.Optimize()}, Output: []assetbuild.Plugin{fixtureModelOutput{}},
	}}}
	options := assetbuild.Options{Project: project, Identity: "geometry-pipeline-fixture"}

	first, err := assetbuild.Build(t.Context(), config, options)
	if err != nil || first.Executed != 4 || first.Written != 1 {
		t.Fatal(first, err)
	}

	file := filepath.Join(project, "generated/model.kmodel")

	//nolint:gosec // Fixed generated path under this test's private project.
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}

	model, err := modeldata.Decode(data)
	if err != nil || model.Source != "assets/scene.glb" || len(model.Parts) != 2 {
		t.Fatalf("source=%q parts=%d error=%v", model.Source, len(model.Parts), err)
	}

	warm, err := assetbuild.Build(t.Context(), config, options)
	if err != nil || warm.Executed != 0 || warm.Reused != 4 {
		t.Fatal(warm, err)
	}

	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}

	repair, err := assetbuild.Build(t.Context(), config, options)
	if err != nil || repair.Executed != 0 || repair.Written != 1 {
		t.Fatal(repair, err)
	}

	if err := os.WriteFile(source, []byte("broken GLB"), 0600); err != nil {
		t.Fatal(err)
	}

	if _, err := assetbuild.Build(t.Context(), config, options); err == nil {
		t.Fatal("accepted invalid changed source")
	}

	//nolint:gosec // Fixed generated path under this test's private project.
	after, err := os.ReadFile(file)
	if err != nil || !bytes.Equal(data, after) {
		t.Fatal("failed build damaged publication", err)
	}
}

// fixtureModelOutput reads the final resource after deferred mesh stages. The
// live level processors consume that same resource; this test only publishes its
// bytes so it can inspect geometry, cache reuse and transactional output repair.
type fixtureModelOutput struct{}

func (fixtureModelOutput) Name() string { return "fixture-model-output" }

func (fixtureModelOutput) Plan(plan *assetbuild.Plan) error {
	return plan.Expand(assetbuild.Expansion{
		ID: "fixture-model-output/discover",
		Run: func(_ *assetbuild.Context, child *assetbuild.Plan) error {
			if len(child.Resources) != 1 {
				return errFixtureModelCount
			}

			model := child.Resources[0].Ref

			refs, err := child.Add(assetbuild.Action{
				ID: "fixture-model-output", Processor: "fixture-model-output", Revision: "1",
				Inputs:  []assetbuild.Ref{model},
				Outputs: []assetbuild.Slot{{Name: "files", Schema: "karty.files@1"}},
				Run: func(ctx *assetbuild.Context) (assetbuild.Result, error) {
					data, err := ctx.Input(model)
					if err != nil {
						return nil, err
					}

					bundle, err := assetbuild.EncodeBundle(assetbuild.Bundle{"model.kmodel": data})
					if err != nil {
						return nil, err
					}

					return assetbuild.Result{"files": bundle}, nil
				},
			})
			if err != nil {
				return err
			}

			child.Publish(refs[0])

			return nil
		},
	})
}
