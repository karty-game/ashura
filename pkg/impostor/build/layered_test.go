//go:build !js

package impostorbuild_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/karty-game/ashura/internal/impostordata"
	compiler "github.com/karty-game/ashura/internal/modelbuild"
	"github.com/karty-game/ashura/internal/modelbuild/testfixture"
	"github.com/karty-game/ashura/pkg/assetbuild"
	impostorbuild "github.com/karty-game/ashura/pkg/impostor/build"
	modelbuild "github.com/karty-game/ashura/pkg/model/build"
)

func TestLayeredPublicBakeAndCachedIntervalBudget(t *testing.T) {
	t.Parallel()
	// The retained scene has heterogeneous roughness; this pipeline explicitly
	// requires a uniform value while retaining its textured mirrored geometry.
	source := testfixture.Scene()
	source = bytes.ReplaceAll(source, []byte(`"roughnessFactor":0.8`), []byte(`"roughnessFactor":0.6`))

	prepared, err := compiler.Compile(source, "scene.glb")
	if err != nil {
		t.Fatal(err)
	}

	encoded, diagnostics, err := impostorbuild.BakeWithReport(
		t.Context(),
		prepared.Data,
		impostorbuild.Options{Mode: impostorbuild.Layered, Resolution: 32, MaxIntervals: 2},
	)
	if err != nil {
		t.Fatal(err)
	}

	document, err := impostordata.Decode(encoded)
	if err != nil || document.Mode != impostordata.ModeLayered || document.Azimuths != 2 || document.MipLevels != 1 ||
		diagnostics.MaximumIntervals != 2 {
		t.Fatalf("public layered bake mismatch %+v: %v", diagnostics, err)
	}

	project := t.TempDir()
	if err := os.Mkdir(filepath.Join(project, "assets"), 0750); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(project, "assets/scene.glb"), source, 0600); err != nil {
		t.Fatal(err)
	}

	config := assetbuild.Config{Output: "generated",
		Pipelines: []assetbuild.Pipeline{{Name: "fixture",
			Input: []string{"assets/*.glb"},
		}},
	}
	options := assetbuild.Options{Project: project, Identity: "layered-budget-fixture"}

	for _, test := range []struct{ intervals, executed, reused int }{{2, 2, 0}, {2, 0, 2}, {4, 1, 1}, {2, 0, 2}} {
		config.Pipelines[0].Import = []assetbuild.Plugin{
			modelbuild.GLBWithOptions(modelbuild.GLBOptions{}),
			impostorbuild.Cards(impostorbuild.Options{Mode: impostorbuild.Layered, Resolution: 32, MaxIntervals: test.intervals}),
		}

		report, err := assetbuild.Build(t.Context(), config, options)
		if err != nil || report.Executed != test.executed || report.Reused != test.reused {
			t.Fatalf("interval budget%d cache report%+v: %v", test.intervals, report, err)
		}
	}
}
