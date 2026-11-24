//go:build !js

package impostorbuild_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/karty-game/ashura/internal/impostordata"
	"github.com/karty-game/ashura/internal/ktx2data"
	"github.com/karty-game/ashura/internal/modelbuild/testfixture"
	"github.com/karty-game/ashura/internal/texturedata"
	"github.com/karty-game/ashura/pkg/assetbuild"
	impostorbuild "github.com/karty-game/ashura/pkg/impostor/build"
	modelbuild "github.com/karty-game/ashura/pkg/model/build"
	"github.com/karty-game/ashura/pkg/texture"
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
			Import: []assetbuild.Plugin{modelbuild.GLBWithOptions(modelbuild.GLBOptions{}), impostorbuild.Cards(settings)},
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
	config.Pipelines[0].Import = []assetbuild.Plugin{modelbuild.GLBWithOptions(modelbuild.GLBOptions{}), impostorbuild.Cards(settings)}

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

	settings.AtlasFormat = texture.ETC2RGBA
	config.Pipelines[0].Import = []assetbuild.Plugin{modelbuild.GLBWithOptions(modelbuild.GLBOptions{}), impostorbuild.Cards(settings)}
	checkMobileCards(t, config, options)

	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}

	removed, err := assetbuild.Build(t.Context(), config, options)
	if err != nil || removed.Removed != 4 {
		t.Fatalf("removed model retained generated impostor %+v: %v", removed, err)
	}
}

func checkMobileCards(t *testing.T, config assetbuild.Config, options assetbuild.Options) {
	t.Helper()

	report, err := assetbuild.Build(t.Context(), config, options)
	if err != nil || report.Executed != 1 || report.Reused != 1 {
		t.Fatal("mobile format did not invalidate only capture output", report, err)
	}

	capture, err := impostordata.Decode(readGenerated(t, options.Project, "impostors/impostor_AssetsScene.kimp"))
	if err != nil || capture.Storage != impostordata.StorageETC2RGBA {
		t.Fatal("mobile capture storage", capture.Storage, err)
	}

	for index, atlas := range [][]byte{capture.Color, capture.NormalDepth} {
		image, err := ktx2data.Decode(atlas)
		if err != nil || image.Format != texture.ETC2RGBA || len(image.Levels) != 2 ||
			(image.ColorSpace == texture.SRGB) != (index == 0) {
			t.Fatal("mobile native blocks changed mip or color/normal transfer", image, err)
		}
	}

	report, err = assetbuild.Build(t.Context(), config, options)
	if err != nil || report.Executed != 0 || report.Reused != 2 {
		t.Fatal("warm mobile capture repeated baking", report, err)
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
