//go:build !js

package impostorbuild_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/karty-game/karty-webgpu/internal/impostordata"
	compiler "github.com/karty-game/karty-webgpu/internal/modelbuild"
	"github.com/karty-game/karty-webgpu/internal/modelbuild/testfixture"
	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
	impostorbuild "github.com/karty-game/karty-webgpu/pkg/impostor/build"
	modelbuild "github.com/karty-game/karty-webgpu/pkg/model/build"
)

func TestPublicBakeDefaultsToOctahedralAndPreservesLegacyAngularSettings(t *testing.T) {
	t.Parallel()

	document, source := testfixture.Triangle()

	model, err := compiler.Compile(testfixture.GLB(document, source), "assets/triangle.glb")
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name              string
		options           impostorbuild.Options
		mode              impostordata.Mode
		resolution, views uint32
	}{
		{name: "default", mode: impostordata.ModeOctahedral, resolution: 80, views: 64},
		{name: "legacy-angular", options: impostorbuild.Options{Azimuths: 4, Elevations: 3}, mode: impostordata.ModeAngular, resolution: 128, views: 12},
		{name: "explicit-angular", options: impostorbuild.Options{Mode: impostorbuild.Angular}, mode: impostordata.ModeAngular, resolution: 128, views: 24},
		{name: "explicit-octahedral", options: impostorbuild.Options{Mode: impostorbuild.Octahedral, FrameGrid: 4}, mode: impostordata.ModeOctahedral, resolution: 80, views: 16},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			data, err := impostorbuild.Bake(t.Context(), model.Data, test.options)
			if err != nil {
				t.Fatal(err)
			}

			decoded, err := impostordata.Decode(data)
			if err != nil || decoded.Mode != test.mode || decoded.Resolution != test.resolution || len(decoded.Views) != int(test.views) {
				t.Fatalf("mode resolution mismatch mode%v res%d views%d err%v", decoded.Mode, decoded.Resolution, len(decoded.Views), err)
			}
		})
	}

	for _, options := range []impostorbuild.Options{
		{Mode: "invalid"}, {Mode: "axial"}, {Mode: "axial-planes"}, {Mode: "surface"},
		{Mode: impostorbuild.Octahedral, Azimuths: 4}, {Mode: impostorbuild.Angular, FrameGrid: 4},
		{FrameGrid: 3}, {FrameGrid: 9}, {Resolution: 82},
	} {
		if _, err := impostorbuild.Bake(t.Context(), model.Data, options); err == nil {
			t.Fatalf("accepted incompatible mode settings %+v", options)
		}
	}
}

func TestModeSwitchCachesCapturesWithoutRecompilingModel(t *testing.T) {
	t.Parallel()

	project := t.TempDir()
	if err := os.Mkdir(filepath.Join(project, "assets"), 0o750); err != nil {
		t.Fatal(err)
	}

	document, source := testfixture.Triangle()
	if err := os.WriteFile(filepath.Join(project, "assets/scene.glb"), testfixture.GLB(document, source), 0o600); err != nil {
		t.Fatal(err)
	}

	settings := impostorbuild.Options{
		Mode:       impostorbuild.Octahedral,
		FrameGrid:  4,
		Include:    []string{"assets/**"},
		Resolution: 32,
		Padding:    2,
		MipLevels:  2,
	}
	plugin := impostorbuild.Cards(settings)
	settings.Mode, settings.FrameGrid, settings.Include[0] = impostorbuild.Angular, 8, "missing/**"
	config := assetbuild.Config{Output: "generated",
		Pipelines: []assetbuild.Pipeline{{Name: "fixture",
			Input:  []string{"assets/*.glb"},
			Import: []assetbuild.Plugin{modelbuild.GLBWithOptions(modelbuild.GLBOptions{}), plugin},
		}},
	}
	options := assetbuild.Options{Project: project, Identity: "mode-cache-fixture"}

	first, err := assetbuild.Build(t.Context(), config, options)
	if err != nil || first.Executed != 2 {
		t.Fatalf("initial mode build %+v: %v", first, err)
	}

	prepared, err := impostordata.Decode(readGenerated(t, project, "impostors/impostor_AssetsScene.kimp"))
	if err != nil || (prepared.Mode != impostordata.ModeOctahedral || prepared.Azimuths != 4) {
		t.Fatalf("caller mutation changed selected mode: %v %v", prepared.Mode, err)
	}

	for _, mode := range []string{impostorbuild.Angular, impostorbuild.Octahedral} {
		config.Pipelines[0].Import = []assetbuild.Plugin{
			modelbuild.GLBWithOptions(modelbuild.GLBOptions{}),
			impostorbuild.Cards(
				impostorbuild.Options{Mode: mode, Include: []string{"assets/**"}, Resolution: 32, Padding: 2, MipLevels: 2},
			),
		}

		if mode == impostorbuild.Octahedral {
			config.Pipelines[0].Import[1] = impostorbuild.Cards(
				impostorbuild.Options{Mode: mode, FrameGrid: 4, Include: []string{"assets/**"}, Resolution: 32, Padding: 2, MipLevels: 2},
			)
		}

		report, err := assetbuild.Build(t.Context(), config, options)
		if err != nil {
			t.Fatal(err)
		}

		executed, reused := 1, 1
		if mode == impostorbuild.Octahedral {
			executed, reused = 0, 2
		}

		if report.Executed != executed || report.Reused != reused {
			t.Fatalf("mode%s recompiled original model or lost capturecache: %+v", mode, report)
		}
	}
}
