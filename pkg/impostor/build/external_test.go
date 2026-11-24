//go:build !js

package impostorbuild_test

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/karty-game/ashura/internal/impostordata"
	"github.com/karty-game/ashura/internal/ktx2data"
	"github.com/karty-game/ashura/internal/modelbuild/testfixture"
	"github.com/karty-game/ashura/pkg/assetbuild"
	"github.com/karty-game/ashura/pkg/engine"
	"github.com/karty-game/ashura/pkg/impostor"
	impostorbuild "github.com/karty-game/ashura/pkg/impostor/build"
	modelbuild "github.com/karty-game/ashura/pkg/model/build"
	"github.com/karty-game/ashura/pkg/texture"
)

func TestExternalProfilesShareModelsAndPublishAdmittedNativePayloads(t *testing.T) {
	t.Parallel()
	project := t.TempDir()

	document, data := testfixture.Triangle()
	if err := os.WriteFile(filepath.Join(project, "scene.glb"), testfixture.GLB(document, data), 0o600); err != nil {
		t.Fatal(err)
	}

	config := assetbuild.Config{Output: "generated", Pipelines: []assetbuild.Pipeline{{Name: "fixture",
		Input: []string{"scene.glb"}, Import: []assetbuild.Plugin{modelbuild.GLBWithOptions(modelbuild.GLBOptions{})},
		Process: []assetbuild.Plugin{
			impostorbuild.Cards(
				impostorbuild.Options{Resolution: 16, Azimuths: 4, Elevations: 3, Padding: 4, MipLevels: 2, TextureProfile: "bc"},
			),
			impostorbuild.Cards(
				impostorbuild.Options{
					Resolution:     16,
					Azimuths:       4,
					Elevations:     3,
					Padding:        4,
					MipLevels:      2,
					TextureProfile: "mobile",
					AtlasFormat:    texture.ASTC6x6,
				},
			),
		},
	}}}
	options := assetbuild.Options{Project: project, Identity: "external-impostor-test"}

	report, err := assetbuild.Build(t.Context(), config, options)
	if err != nil || report.Executed != 3 || report.Written != 6 {
		t.Fatal("one shared model and two independent profiles", report, err)
	}

	app := engine.NewApp(engine.AppConfig{})

	t.Cleanup(func() { _ = app.Close() })

	for _, profile := range []string{"bc", "mobile"} {
		path := "texture-profiles/" + profile + "/impostor_Scene.kimp"
		payload := readGenerated(t, project, "public/"+path)

		catalog := string(readGenerated(t, project, "impostors/"+profile+"/impostor_Scene.go"))
		if strings.Contains(catalog, "embed") || !strings.Contains(catalog, path) ||
			!strings.Contains(
				catalog,
				fmt.Sprintf("%x", sha256.Sum256(payload)),
			) || !strings.Contains(catalog, fmt.Sprintf("Size: %d", len(payload))) {
			t.Fatal("external descriptor lost path, size or checksum", catalog)
		}

		capture, err := impostordata.Decode(payload)
		if err != nil || capture.Resolution != 16 || capture.Padding != 4 || capture.MipLevels != 2 {
			t.Fatal("capture detail changed", capture, err)
		}

		color, err := ktx2data.Decode(capture.Color)
		if err != nil || (color.Format == texture.ASTC6x6) != (profile == "mobile") {
			t.Fatal("wrong native profile", color.Format, err)
		}

		card, err := impostor.Prepared(profile, payload).Load(app)
		if err != nil {
			t.Fatal("native storage admission", err)
		}

		if err := card.Unload(); err != nil {
			t.Fatal(err)
		}
	}

	report, err = assetbuild.Build(t.Context(), config, options)
	if err != nil || report.Executed != 0 || report.Reused != 3 {
		t.Fatal("warm profiles were rebuilt", report, err)
	}
}
