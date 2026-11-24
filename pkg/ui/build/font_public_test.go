//go:build !js

package build

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/karty-game/ashura/pkg/assetbuild"
	"github.com/karty-game/ashura/pkg/texture"
)

func TestDefaultFontWASMNeedsNoExecutablesAndReusesCache(t *testing.T) {
	t.Setenv("PATH", "")
	t.Setenv("CC", "compiler-must-not-run")
	t.Setenv("CXX", "compiler-must-not-run")

	ctx, cancel := context.WithTimeout(t.Context(), 4*time.Minute)
	defer cancel()

	// Keep the project real even when macOS exposes its temp root through /var.
	project, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	config := assetbuild.Config{Output: "prepared", Pipelines: []assetbuild.Pipeline{{
		Name: "font", Output: []assetbuild.Plugin{DefaultFont()},
	}}}
	options := assetbuild.Options{Project: project, Identity: "wasm-font-probe"}

	if report, err := assetbuild.Build(ctx, config, options); err != nil || report.Executed != 1 {
		t.Fatal("embedded WASM font preparation failed", report, err)
	}

	if report, err := assetbuild.Build(ctx, config, options); err != nil || report.Executed != 0 {
		t.Fatal("unchanged font did not reuse its verified action", report, err)
	}

	options.NoCache = true
	if _, err := assetbuild.Build(ctx, config, options); err != nil {
		t.Fatal("embedded font library could not regenerate offline", err)
	}

	metrics, err := os.ReadFile(filepath.Join(project, "prepared", "glyphs.bin"))
	if err != nil {
		t.Fatal(err)
	}

	atlas, err := os.ReadFile(filepath.Join(project, "prepared", "atlas.rgba"))
	if err != nil {
		t.Fatal(err)
	}

	if err := validateDefaultFont(metrics, atlas); err != nil {
		t.Fatal("published pair differs from qualified WASM profile", err)
	}
}

func TestDefaultFontBC7PublicationKeepsMetricsAndReusesCache(t *testing.T) {
	t.Setenv("PATH", "")

	project, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	config := assetbuild.Config{Output: "prepared", Pipelines: []assetbuild.Pipeline{{
		Name: "font", Output: []assetbuild.Plugin{DefaultFont(DefaultFontOptions{Format: texture.BC7})},
	}}}

	options := assetbuild.Options{Project: project, Identity: "wasm-font-bc7-probe"}
	if report, err := assetbuild.Build(t.Context(), config, options); err != nil || report.Executed != 1 {
		t.Fatal("explicit BC7 font preparation failed", report, err)
	}

	metrics, err := os.ReadFile(filepath.Join(project, "prepared", "glyphs.bin"))
	if err != nil {
		t.Fatal(err)
	}

	atlas, err := os.ReadFile(filepath.Join(project, "prepared", "atlas.bc7"))
	if err != nil {
		t.Fatal(err)
	}

	if err := validateCompressedDefaultFont(metrics, atlas); err != nil {
		t.Fatal("compressed page identity or retained glyph metrics changed", err)
	}

	if report, err := assetbuild.Build(t.Context(), config, options); err != nil || report.Executed != 0 || report.Written != 0 {
		t.Fatal("warm compressed font repeated processing or publication", report, err)
	}

	if _, err := os.Stat(filepath.Join(project, "prepared", "atlas.rgba")); !os.IsNotExist(err) {
		t.Fatal("compressed font published an extra raw page", err)
	}
}
