//go:build !js

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
)

func TestAssetGenerateThenPlayableBuild(t *testing.T) {
	// Only this isolated module disables VCS stamping.
	t.Setenv("GOFLAGS", "-mod=mod -buildvcs=false")
	project := assetRecipeFixture(t)
	recipe := strings.Replace(recipeFixture,
		`import "github.com/karty-game/karty-webgpu/pkg/assetbuild"`,
		`import ("os"; "github.com/karty-game/karty-webgpu/pkg/assetbuild")`, 1)
	recipe = strings.Replace(
		recipe,
		"size := 2",
		`if err := os.WriteFile("invoked-"+string(env.Phase), nil, 0600); err != nil { panic(err) }
        size := 2`,
		1,
	)
	writeWebFixtures(t, project, map[string]string{
		assetConfigFile: recipe,
		"main.go":       "package main\nimport \"example.test/assets/generated/assets\"\nfunc main() { _ = assets.UiAlbedoPage0() }\n",
	})
	t.Chdir(project)

	if err := run([]string{"assets", "generate", ".", "--target=js/wasm"}); err != nil {
		t.Fatal(err)
	}

	before := readRecipeOutputs(t, project)
	stamps := stampRecipeOutputs(t, project, before)

	dist := t.TempDir()
	if err := buildWebPreview(t.Context(), project, dist, ".", webOptions{assets: assetOptions{target: "js/wasm"}}); err != nil {
		t.Fatal(err)
	}

	assertRecipeUnchanged(t, project, before, stamps)

	for _, phase := range []string{"generate", "build"} {
		if _, err := os.Stat(filepath.Join(project, "invoked-"+phase)); err != nil {
			t.Fatalf("recipe did not receive %s phase: %v", phase, err)
		}
	}

	//nolint:gosec // Fixed output name inside this test's private staging directory.
	data, err := os.ReadFile(filepath.Join(dist, wasmFile))
	if err != nil || !bytes.HasPrefix(data, []byte("\x00asm")) {
		t.Fatalf("missing playable WASM output: %v", err)
	}
}

func TestAssetLifecycleRejectsMixedSelectors(t *testing.T) {
	t.Parallel()

	for _, phase := range []assetbuild.Phase{assetbuild.PhaseGenerate, assetbuild.PhaseBuild} {
		for _, args := range [][]string{
			{"./samples/...", "--project=."},
			{"./samples/...", "--source=assets"},
			{"./samples/...", "--output=generated/assets"},
			{"./first", "./second"},
		} {
			if err := runAssetCatalog(t.Context(), args, phase); err == nil {
				t.Fatalf("phase %s accepted %v", phase, args)
			}
		}
	}
}
