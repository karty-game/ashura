//go:build !js

package levelbuild

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
)

func TestNativeLibraryUsesOnlyPinnedUpstreamArchives(t *testing.T) {
	t.Parallel()

	for _, target := range []string{"linux-amd64", "linux-arm64", "darwin-arm64", "windows-amd64"} {
		spec, err := nativeLibrarySpecification(target)
		if err != nil || spec.Version != "29.0.0.0" || len(spec.SHA256) != 64 ||
			!strings.HasPrefix(spec.URL, "https://github.com/gfx-rs/wgpu-native/releases/download/v29.0.0.0/") ||
			!strings.HasPrefix(spec.Executable, "lib/") {
			t.Fatal(target, spec, err)
		}
	}

	if _, err := nativeLibrarySpecification("unsupported"); err == nil {
		t.Fatal("unsupported host accepted")
	}
}

type nativeLibraryProbe struct{}

func (nativeLibraryProbe) Name() string { return "native-library-probe" }
func (nativeLibraryProbe) Plan(plan *assetbuild.Plan) error {
	refs, err := plan.Add(
		assetbuild.Action{
			ID:        "library",
			Processor: "native-library-probe",
			Revision:  "1",
			Outputs:   []assetbuild.Slot{{Name: "files", Schema: "files@1"}},
			Run: func(ctx *assetbuild.Context) (assetbuild.Result, error) {
				library, err := productionNativeLibrary(ctx)
				if err != nil {
					return nil, err
				}

				data, err := json.Marshal(assetbuild.Bundle{"library.txt": []byte(library)})

				return assetbuild.Result{"files": data}, err
			},
		},
	)
	if err == nil {
		plan.Publish(refs[0])
	}

	return err
}

func TestPublicNativeLibraryInstallAndOfflineReuse(t *testing.T) {
	if os.Getenv("KARTY_VERIFY_PUBLIC_TOOLS") != "1" {
		t.Skip("opt-in official native library installation")
	}

	for _, name := range []string{"MISE_DATA_DIR", "MISE_CACHE_DIR", "MISE_CONFIG_DIR", "MISE_STATE_DIR"} {
		t.Setenv(name, t.TempDir())
	}

	t.Setenv("WGPU_NATIVE_PATH", "")
	t.Setenv("MISE_OFFLINE", "0")
	t.Setenv("CC", filepath.Join(t.TempDir(), "must-not-run-compiler"))
	t.Setenv("CXX", filepath.Join(t.TempDir(), "must-not-run-compiler"))

	ctx, cancel := context.WithTimeout(t.Context(), 4*time.Minute)
	defer cancel()

	project, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	config := assetbuild.Config{
		Output:    "prepared",
		Pipelines: []assetbuild.Pipeline{{Name: "probe", Output: []assetbuild.Plugin{nativeLibraryProbe{}}}},
	}

	options := assetbuild.Options{Project: project, Identity: "official-wgpu-native-install"}
	if report, err := assetbuild.Build(ctx, config, options); err != nil || report.Executed != 1 {
		t.Fatal("cold upstream install", report, err)
	}

	t.Setenv("MISE_OFFLINE", "1")

	if report, err := assetbuild.Build(ctx, config, options); err != nil || report.Executed != 0 {
		t.Fatal("warm action reuse", report, err)
	}

	options.NoCache = true
	if report, err := assetbuild.Build(ctx, config, options); err != nil || report.Executed != 1 {
		t.Fatal("offline installed library", report, err)
	}
}
