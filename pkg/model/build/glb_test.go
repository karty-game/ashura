//go:build !js

//nolint:errcheck,forcetypeassert // Type assertions access only the engine-owned fixture schema.
package modelbuild_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/karty-game/karty-webgpu/internal/modelbuild/testfixture"
	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
	modelbuild "github.com/karty-game/karty-webgpu/pkg/model/build"
)

func TestRecipePreparesCachesCatalogAndPrunesRemovedModel(t *testing.T) {
	t.Parallel()

	project, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	if err := os.Mkdir(filepath.Join(project, "assets"), 0o750); err != nil {
		t.Fatal(err)
	}

	source := filepath.Join(project, "assets", "scene.glb")
	if err := os.WriteFile(source, testfixture.Scene(), 0o600); err != nil {
		t.Fatal(err)
	}

	config := assetbuild.Config{Output: "generated",
		Pipelines: []assetbuild.Pipeline{{Name: "fixture",
			Input:  []string{"assets/**"},
			Import: []assetbuild.Plugin{modelbuild.GLB()},
		}},
	}
	options := assetbuild.Options{Project: project, Identity: "model-fixture-tool-v1"}

	first, err := assetbuild.Build(context.Background(), config, options)
	if err != nil {
		t.Fatal(err)
	}

	if first.Executed != 1 || first.Written != 2 {
		t.Fatalf("first build: %#v", first)
	}

	codePath := filepath.Join(project, "generated", "models", "model_AssetsScene.go")

	code, err := os.ReadFile(codePath)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(code), "func AssetsScene() model.Asset") ||
		!strings.Contains(string(code), `model.Prepared("assets/scene.glb"`) {
		t.Fatalf("wrong named declaration: %s", code)
	}

	second, err := assetbuild.Build(context.Background(), config, options)
	if err != nil {
		t.Fatal(err)
	}

	if second.Executed != 0 || second.Reused != 1 || second.Unchanged != 2 {
		t.Fatalf("unchanged build did not reuse conversion: %#v", second)
	}

	config.Pipelines[0].Import = []assetbuild.Plugin{modelbuild.GLBWithOptions(modelbuild.GLBOptions{MaxTextureDimension: 1})}

	resized, err := assetbuild.Build(context.Background(), config, options)
	if err != nil || resized.Executed != 1 {
		t.Fatal("texture cap reused the original-resolution action", resized, err)
	}

	warm, err := assetbuild.Build(context.Background(), config, options)
	if err != nil || warm.Executed != 0 || warm.Reused != 1 {
		t.Fatal("texture cap did not cache independently", warm, err)
	}

	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}

	third, err := assetbuild.Build(context.Background(), config, options)
	if err != nil {
		t.Fatal(err)
	}

	if third.Removed != 2 {
		t.Fatalf("removed model retained generated files: %#v", third)
	}
}

func TestMixedImageAndModelRecipeBuildsDistinctPackages(t *testing.T) {
	t.Parallel()

	project, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	if err := os.Mkdir(filepath.Join(project, "assets"), 0o750); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(project, "assets", "scene.glb"), testfixture.Scene(), 0o600); err != nil {
		t.Fatal(err)
	}

	document, data := testfixture.Triangle()
	data = testfixture.EmbedTexture(document, data)
	view := document["bufferViews"].([]any)[4].(map[string]any)

	start, length := view["byteOffset"].(int), view["byteLength"].(int)
	if err := os.WriteFile(filepath.Join(project, "assets", "scene.png"), data[start:start+length], 0o600); err != nil {
		t.Fatal(err)
	}

	config := assetbuild.Config{Output: "generated/assets",
		Pipelines: []assetbuild.Pipeline{{Name: "fixture",
			Input: []string{"assets/**"},
			Import: []assetbuild.Plugin{
				assetbuild.Images(), assetbuild.Mipmaps(), assetbuild.KTX2(), assetbuild.GoCatalog(), modelbuild.GLB(),
			},
		}},
	}
	if _, err := assetbuild.Build(
		context.Background(),
		config,
		assetbuild.Options{Project: project, Identity: "mixed-model-fixture-v1"},
	); err != nil {
		t.Fatal(err)
	}

	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}

	module := fmt.Sprintf(
		"module github.com/karty-game/karty-webgpu/internal/model-recipe-fixture\n\ngo 1.27.0\n\nrequire github.com/karty-game/karty-webgpu v0.0.0\n\nreplace github.com/karty-game/karty-webgpu => %q\n",
		root,
	)
	if err := os.WriteFile(filepath.Join(project, "go.mod"), []byte(module), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	command := exec.CommandContext(ctx, "go", "build", "-mod=mod", "-buildvcs=false", "./generated/...")
	command.Dir = project

	command.Env = append(os.Environ(), "GOWORK=off")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("mixed generated packages did not compile: %v\n%s", err, output)
	}
}
