//go:build !js

package impostorbuild_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/karty-game/karty-webgpu/internal/impostordata"
	"github.com/karty-game/karty-webgpu/internal/ktx2data"
	"github.com/karty-game/karty-webgpu/internal/modelbuild/testfixture"
	"github.com/karty-game/karty-webgpu/internal/texturedata"
	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
	impostorbuild "github.com/karty-game/karty-webgpu/pkg/impostor/build"
	modelbuild "github.com/karty-game/karty-webgpu/pkg/model/build"
)

func TestCardsConsumesCachedModelAndPublishesIndependentCatalog(t *testing.T) {
	t.Parallel()

	project := t.TempDir()
	if err := os.Mkdir(filepath.Join(project, "assets"), 0o750); err != nil {
		t.Fatal(err)
	}

	document, data := testfixture.Triangle()

	source := filepath.Join(project, "assets/scene.glb")
	if err := os.WriteFile(source, testfixture.GLB(document, data), 0o600); err != nil {
		t.Fatal(err)
	}

	settings := impostorbuild.Options{Resolution: 16, Azimuths: 4, Elevations: 3, Padding: 2, MipLevels: 2}
	config := assetbuild.Config{Output: "generated",
		Pipelines: []assetbuild.Pipeline{{Name: "fixture",
			Input:  []string{"assets/**"},
			Import: []assetbuild.Plugin{modelbuild.GLB(), impostorbuild.Cards(settings)},
		}},
	}
	options := assetbuild.Options{Project: project, Identity: "impostor-fixture@1"}

	first, err := assetbuild.Build(t.Context(), config, options)
	if err != nil || first.Executed != 2 || first.Written != 4 {
		t.Fatalf("first build %+v: %v", first, err)
	}

	model := readGenerated(t, project, "models/model_AssetsScene.kmodel")
	prepared := readGenerated(t, project, "impostors/impostor_AssetsScene.kimp")

	capture, err := impostordata.Decode(prepared)
	if err != nil || capture.SourceTriangles != 1 || len(capture.Views) != 12 {
		t.Fatalf("prepared capture %+v: %v", capture, err)
	}

	for _, atlas := range [][]byte{capture.Color, capture.NormalDepth} {
		image, decodeErr := ktx2data.Decode(atlas)
		if decodeErr != nil || capture.Storage != impostordata.StorageBC1 || image.Format != texturedata.BC1 || !ktx2data.BC1Alpha(atlas) {
			t.Fatal("view pipeline did not publish native BC1 coverage/storage", decodeErr, capture.Storage, image.Format)
		}
	}

	code := readGenerated(t, project, "impostors/impostor_AssetsScene.go")
	if !strings.Contains(string(code), "func AssetsScene() impostor.Asset") ||
		!strings.Contains(string(code), `impostor.Prepared("assets/scene.glb"`) {
		t.Fatalf("named catalog lost stable source binding: %s", code)
	}

	second, err := assetbuild.Build(t.Context(), config, options)
	if err != nil || second.Executed != 0 || second.Reused != 2 || second.Unchanged != 4 {
		t.Fatalf("warm capture recomputed %+v: %v", second, err)
	}

	settings.Resolution = 32
	config.Pipelines[0].Import = []assetbuild.Plugin{modelbuild.GLB(), impostorbuild.Cards(settings)}

	changed, err := assetbuild.Build(t.Context(), config, options)
	if err != nil || changed.Executed != 1 || changed.Reused != 1 {
		t.Fatalf("capture settings recompiled original model %+v: %v", changed, err)
	}

	if !bytes.Equal(model, readGenerated(t, project, "models/model_AssetsScene.kmodel")) {
		t.Fatal("impostor settings modified original prepared model")
	}

	if bytes.Equal(prepared, readGenerated(t, project, "impostors/impostor_AssetsScene.kimp")) {
		t.Fatal("capture resolution did not affect prepared resource")
	}

	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}

	removed, err := assetbuild.Build(t.Context(), config, options)
	if err != nil || removed.Removed != 4 {
		t.Fatalf("removed model retained generated impostor %+v: %v", removed, err)
	}
}

func readGenerated(t *testing.T, root, name string) []byte {
	t.Helper()
	//nolint:gosec // The test-owned relative artifact is beneath t.TempDir.
	data, err := os.ReadFile(filepath.Join(root, "generated", filepath.FromSlash(name)))
	if err != nil {
		t.Fatal(err)
	}

	return data
}
