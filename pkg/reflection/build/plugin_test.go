//go:build !js

package reflectionbuild

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/karty-game/ashura/pkg/assetbuild"
)

func TestBakeCacheTracksItsRecipeExecutable(t *testing.T) {
	t.Parallel()

	project := t.TempDir()
	copyRoomSources(t, project)

	config := assetbuild.Config{Output: "generated/assets", Pipelines: []assetbuild.Pipeline{{
		Name: "reflections", Import: []assetbuild.Plugin{Scene(fixtureOptions())},
	}}}

	options := assetbuild.Options{Project: project, Identity: "sample-panorama-cache-test"}
	for attempt := range 2 {
		report, err := assetbuild.Build(t.Context(), config, options)
		if err != nil {
			t.Fatal(err)
		}

		if report.Executed != 2*(1-attempt) || report.Reused != 2*attempt {
			t.Fatalf("attempt %d: executed %d, reused %d", attempt, report.Executed, report.Reused)
		}
	}

	// The recipe host executable fingerprints library algorithm changes.
	options.Identity = "reflection-baker-version-two"

	report, err := assetbuild.Build(t.Context(), config, options)
	if err != nil {
		t.Fatal(err)
	}

	if report.Executed != 2 || report.Reused != 0 {
		t.Fatalf("renderer edit: executed %d, reused %d", report.Executed, report.Reused)
	}
}

func copyRoomSources(t *testing.T, project string) {
	t.Helper()

	for _, relative := range []string{"assets/room.json", "assets/chunks/room.json"} {
		// #nosec G304 -- Relative paths are the two fixed, maintained sample fixture files above.
		data, err := os.ReadFile(filepath.Join("testdata", relative))
		if err != nil {
			t.Fatal(err)
		}

		name := filepath.Join(project, relative)
		if err := os.MkdirAll(filepath.Dir(name), 0o750); err != nil {
			t.Fatal(err)
		}

		// #nosec G703 -- Destination is confined to this test's isolated temporary project.
		if err := os.WriteFile(name, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func fixtureOptions() SceneOptions {
	return SceneOptions{Name: "Room", Source: "assets/room.json", Chunks: "assets/chunks/*.json"}
}
