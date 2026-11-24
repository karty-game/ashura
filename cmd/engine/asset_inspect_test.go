//go:build !js

package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/karty-game/ashura/pkg/assetbuild"
)

func TestAssetInspectDefault(t *testing.T) {
	t.Parallel()

	project := assetRecipeFixture(t)
	if err := os.Remove(filepath.Join(project, assetConfigFile)); err != nil {
		t.Fatal(err)
	}

	writeWebFixtures(t, project, map[string]string{"assets/unrecognized.webp": "unsupported"})

	var out bytes.Buffer
	if err := inspectAssets(t.Context(), []string{"--project", project}, &out); err != nil {
		t.Fatal(err)
	}

	for _, expected := range []string{
		"Pipeline textures → generated/assets", "assets.UiRed()", "generated/assets/ui/red.ktx2",
		"Skipped by PNG/JPEG importer: \"assets/unrecognized.webp\"",
	} {
		if !strings.Contains(out.String(), expected) {
			t.Errorf("missing %q in:\n%s", expected, &out)
		}
	}

	if _, err := os.Stat(filepath.Join(project, "generated")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("inspection created build state: %v", err)
	}

	if err := prepareProjectAssets(t.Context(), project, assetOptions{log: &out}); err != nil {
		t.Fatal(err)
	}

	files := readRecipeOutputs(t, project)
	if !bytes.Contains(files["assets.go"], []byte("func UiRed()")) || len(files["ui/red.ktx2"]) == 0 {
		t.Fatal("built catalog disagrees with inspection")
	}

	stamps := stampRecipeOutputs(t, project, files)
	if err := inspectAssets(t.Context(), []string{project}, &out); err != nil {
		t.Fatal(err)
	}

	assertRecipeUnchanged(t, project, files, stamps)

	writeWebFixtures(t, project, map[string]string{"assets/ui-red.png": "the collision precedes image decoding"})

	err := inspectAssets(t.Context(), []string{project}, &out)
	if err == nil || !strings.Contains(err.Error(), `"assets/ui-red.png" and "assets/ui/red.png" both generate "UiRed"`) {
		t.Fatalf("missing specific naming collision: %v", err)
	}
}

func TestAssetInspectRecipe(t *testing.T) {
	t.Parallel()

	project := assetRecipeFixture(t)
	recipe := strings.Replace(recipeFixture, `"assets/**"`, `"assets/**", "optional/**"`, 1)
	recipe = strings.Replace(recipe, `"ui/**"`, `"ui/**", "typo/**"`, 1)
	writeWebFixtures(t, project, map[string]string{assetConfigFile: recipe})

	var out bytes.Buffer

	args := []string{project, "--phase", "build", "--profile", "release", "--target", "js/wasm"}
	if err := inspectAssets(t.Context(), args, &out); err != nil {
		t.Fatal(err)
	}

	for _, expected := range []string{
		"build, profile release, target js/wasm",
		`Atlas "ui": 2 selected`,
		`atlas "ui" pattern "typo/**" matched no images`,
		`input pattern "optional/**" matched no files`,
		`"assets/ui/red.png" (ID "ui/red", albedo) → atlas "ui" → assets.UiRedRegion()`,
	} {
		if !strings.Contains(out.String(), expected) {
			t.Errorf("missing %q in:\n%s", expected, &out)
		}
	}

	if _, err := os.Stat(filepath.Join(project, "generated")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("inspection created build state: %v", err)
	}

	if err := prepareProjectAssets(t.Context(), project, assetOptions{
		phase: assetbuild.PhaseBuild, profile: "release", target: "js/wasm", log: &out,
	}); err != nil {
		t.Fatal(err)
	}

	files := readRecipeOutputs(t, project)
	if !bytes.Contains(files["assets.go"], []byte("func UiRedRegion()")) || len(files["atlas.json"]) == 0 {
		t.Fatal("built atlas catalog disagrees with inspection")
	}
}
