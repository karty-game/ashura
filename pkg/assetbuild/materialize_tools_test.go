//go:build !js

package assetbuild

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestMaterializePublishedHostPackages(t *testing.T) {
	t.Parallel()

	var release managedToolRelease
	if err := json.Unmarshal(materializeReleaseJSON, &release); err != nil {
		t.Fatal(err)
	}

	if release.Availability != "published" || release.Version != "2.0.0" || !release.complete() {
		t.Fatal("default must select the existing complete Materialize 2.0.0 release", release)
	}

	for _, target := range []string{"darwin-arm64", "linux-amd64", "linux-arm64", "windows-amd64"} {
		parts := strings.Split(target, "-")

		spec, err := materializeToolSpecification(parts[0], parts[1])
		if err != nil || validateManagedTool(spec) != nil || spec.Version != materializeToolVersion ||
			!strings.HasSuffix(spec.URL, "/materialize-2.0.0-"+target+".zip") ||
			strings.HasSuffix(spec.Executable, ".exe") != (parts[0] == "windows") {
			t.Fatal("wrong host archive or executable", target, spec)
		}
	}

	for _, target := range [][2]string{{"darwin", "amd64"}, {"windows", "arm64"}, {"linux", "386"}} {
		if _, err := materializeToolSpecification(target[0], target[1]); !errors.Is(err, errManagedTool) {
			t.Fatal("unpublished target did not fail explicitly", target, err)
		}
	}
}

func TestMaterializeDefaultUsesModulePinAndMiseInstallation(t *testing.T) {
	materializeTestRelease(t)

	mise := newManagedMiseFixture(t)
	_, config, options, material := materializeFixture(t)

	spec, err := materializeToolSpecification(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Fatal(err)
	}

	mise.write(t, filepath.Join(mise.root, filepath.FromSlash(spec.Executable)), mise.binary)
	material.Executable = ""
	config.Plugins[1] = Materialize(material)

	t.Setenv("MATERIALIZE_CLI", "")

	if _, err := Build(t.Context(), config, options); err != nil {
		t.Fatal("default module-owned Materialize resolution failed", err)
	}

	assertMaterializeCalls(t, material, 1)

	calls := mise.calls(t)
	if len(calls) != 2 || !strings.Contains(calls[0][4], "url="+spec.URL) ||
		!strings.Contains(calls[0][4], "checksum=sha256:"+spec.SHA256) {
		t.Fatal("default Materialize did not request its public pinned artifact", calls)
	}

	if _, err := os.Stat(filepath.Join(mise.root, "notices", "Materialize-LICENSE")); err != nil {
		t.Fatal("default resolver did not retain mise's full installation layout", err)
	}

	warm, err := Build(t.Context(), config, options)
	if err != nil || warm.Executed != 0 || len(mise.calls(t)) != 2 {
		t.Fatal("unchanged default Materialize did not reuse its action", warm, err)
	}

	options.NoCache = true
	if _, err := Build(t.Context(), config, options); err != nil {
		t.Fatal("forced processing did not resolve the installed module again", err)
	}

	assertMaterializeCalls(t, material, 2)

	if len(mise.calls(t)) != 4 {
		t.Fatal("forced processing did not use mise install/where exactly once", mise.calls(t))
	}
}

// Tests substitute only the module declaration; fake mise observes the exact
// declared URL/digest. Public install qualification uses unmodified release pins.
func materializeTestRelease(t *testing.T) {
	t.Helper()

	release := managedToolRelease{Schema: 1, Name: materializeToolName, Version: materializeToolVersion,
		Availability: "published", Artifacts: map[string]managedToolArtifact{}}

	for _, target := range []string{"linux-amd64", "linux-arm64", "darwin-arm64", "windows-amd64"} {
		executable := "bin/materialize-cli"
		if target == "windows-amd64" {
			executable += ".exe"
		}

		release.Artifacts[target] = managedToolArtifact{URL: "https://tools.invalid/materialize-" + target + ".zip",
			SHA256: strings.Repeat("a", 64), Format: managedZip, Executable: executable}
	}

	previous := materializeReleaseJSON

	t.Cleanup(func() { materializeReleaseJSON = previous })

	var err error

	materializeReleaseJSON, err = json.Marshal(release)
	if err != nil {
		t.Fatal(err)
	}
}

func TestMaterializeExecutableOverridesRemainTracked(t *testing.T) {
	_, config, options, material := materializeFixture(t)
	t.Setenv("MATERIALIZE_CLI", filepath.Join(options.Project, "must-not-run"))

	if _, err := Build(t.Context(), config, options); err != nil {
		t.Fatal("environment override replaced explicit executable", err)
	}

	t.Setenv("MATERIALIZE_CLI", material.Executable)
	material.Executable = ""

	config.Plugins[1] = Materialize(material)
	if _, err := Build(t.Context(), config, options); err != nil {
		t.Fatal("tracked environment executable was not selected", err)
	}

	assertMaterializeCalls(t, material, 2)
	t.Setenv("MATERIALIZE_CLI", filepath.Join(options.Project, "must-not-run"))

	if _, err := Build(t.Context(), config, options); err == nil {
		t.Fatal("changed executable environment reused the old result")
	}
}
