//go:build !js

package assetbuild

import (
	"errors"
	"strings"
	"testing"
)

func TestModuleReleaseRequiresCompletePublishedHostMatrix(t *testing.T) {
	t.Parallel()

	release := managedToolRelease{Schema: 1, Name: "fixture", Version: "1.0.0",
		Artifacts: map[string]managedToolArtifact{}}

	for _, target := range []string{"linux-amd64", "linux-arm64", "darwin-arm64", "windows-amd64"} {
		executable := "bin/fixture"
		if target == "windows-amd64" {
			executable += ".exe"
		}

		release.Artifacts[target] = managedToolArtifact{URL: "https://tools.invalid/" + target + ".zip",
			SHA256: strings.Repeat("a", 64), Format: managedZip, Executable: executable}
	}

	for _, availability := range []string{"", "candidate", "pending-publication"} {
		release.Availability = availability
		if _, err := release.forTarget("linux-amd64"); !errors.Is(err, errManagedTool) {
			t.Fatal("unpublished candidates must not become consumer dependencies", availability, err)
		}
	}

	release.Availability = "published"
	if spec, err := release.forTarget("windows-amd64"); err != nil || spec.Executable != "bin/fixture.exe" {
		t.Fatal("complete release did not select its native package", spec, err)
	}

	if _, err := release.forTarget("unsupported"); !errors.Is(err, errManagedTool) {
		t.Fatal("unsupported host selected another platform's package", err)
	}

	delete(release.Artifacts, "linux-arm64")

	if _, err := release.forTarget("linux-amd64"); !errors.Is(err, errManagedTool) {
		t.Fatal("partial host release was admitted", err)
	}
}
