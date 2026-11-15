//go:build !js

package modelbuild_test

import (
	"context"
	"crypto/sha256"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/karty-game/karty-webgpu/internal/lightmapdata"
	"github.com/karty-game/karty-webgpu/internal/modelbuild/testfixture"
	"github.com/karty-game/karty-webgpu/internal/modeldata"
	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
	modelbuild "github.com/karty-game/karty-webgpu/pkg/model/build"
)

func TestLightmappedGLBPublishesMatchingCachedArtifacts(t *testing.T) {
	t.Setenv("PATH", "")
	t.Setenv("CC", "compiler-must-not-run")
	t.Setenv("CXX", "compiler-must-not-run")

	project, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	if err := os.Mkdir(filepath.Join(project, "assets"), 0o750); err != nil {
		t.Fatal(err)
	}

	document, data := testfixture.Triangle()

	source := filepath.Join(project, "assets", "scene.glb")
	if err := os.WriteFile(source, testfixture.GLB(document, data), 0o600); err != nil {
		t.Fatal(err)
	}

	config := assetbuild.Config{Output: "generated",
		Pipelines: []assetbuild.Pipeline{{Name: "fixture",
			Input: []string{"assets/*.glb"},
			Import: []assetbuild.Plugin{
				modelbuild.LightmappedGLB(
					modelbuild.LightmapOptions{
						Resolution:          64,
						RNM:                 true,
						PrecomputeTransport: true,
						TransportSamples:    16,
					},
				),
			},
		}},
	}
	options := assetbuild.Options{Project: project, Identity: "lightmap-test@1"}

	first, err := assetbuild.Build(context.Background(), config, options)
	if err != nil {
		t.Fatal(err)
	}

	if first.Executed != 1 || first.Written != 4 {
		t.Fatalf("unexpected first publication: %#v", first)
	}

	prepared, err := os.ReadFile(filepath.Join(project, "generated", "models", "model_AssetsScene.kmodel"))
	if err != nil {
		t.Fatal(err)
	}

	sidecar, err := os.ReadFile(filepath.Join(project, "generated", "models", "lightmap_AssetsScene.klightmap"))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := modeldata.Decode(prepared); err != nil {
		t.Fatal(err)
	}

	bake, err := lightmapdata.Decode(sidecar)
	if err != nil {
		t.Fatal(err)
	}

	if !bake.RNM() || !bake.Transport() || bake.GISamples != 16 {
		t.Fatal("RNM option did not reach prepared artifact")
	}

	if bake.ModelSHA256 != sha256.Sum256(prepared) {
		t.Fatal("sidecar not bound to remapped model")
	}

	code, err := os.ReadFile(filepath.Join(project, "generated", "models", "lightmap_AssetsScene.go"))
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(code), "func AssetsSceneLightmap() lightmap.Declaration") {
		t.Fatalf("missing generated declaration: %s", code)
	}

	second, err := assetbuild.Build(context.Background(), config, options)
	if err != nil {
		t.Fatal(err)
	}

	if second.Executed != 0 || second.Reused != 1 || second.Unchanged != 4 {
		t.Fatalf("warm artifact changed: %#v", second)
	}
}
