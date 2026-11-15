//go:build !js

package assetbuild

import "fmt"

type managedToolRelease struct {
	Schema       int                            `json:"schema"`
	Name         string                         `json:"name"`
	Version      string                         `json:"version"`
	Availability string                         `json:"availability"`
	Artifacts    map[string]managedToolArtifact `json:"artifacts"`
}

type managedToolArtifact struct {
	URL             string   `json:"url"`
	SHA256          string   `json:"sha256"`
	Format          string   `json:"format"`
	Executable      string   `json:"executable"`
	Components      []string `json:"components,omitempty"`
	StripComponents int      `json:"stripComponents,omitempty"`
}

func (release managedToolRelease) forTarget(target string) (ManagedToolSpec, error) {
	if release.Schema != 1 || release.Availability != "published" || !release.complete() {
		return ManagedToolSpec{}, fmt.Errorf("%s %s requires a verified public four-host release: %w",
			release.Name, release.Version, errManagedTool)
	}

	artifact, exists := release.Artifacts[target]
	if !exists {
		return ManagedToolSpec{}, fmt.Errorf("%s has no package for %s: %w", release.Name, target, errManagedTool)
	}

	return release.spec(artifact), nil
}

func (release managedToolRelease) spec(artifact managedToolArtifact) ManagedToolSpec {
	return ManagedToolSpec{Name: release.Name, Version: release.Version,
		URL: artifact.URL, SHA256: artifact.SHA256, Format: artifact.Format,
		Executable: artifact.Executable, Components: artifact.Components, StripComponents: artifact.StripComponents}
}

func (release managedToolRelease) complete() bool {
	for _, target := range []string{"linux-amd64", "linux-arm64", "darwin-arm64", "windows-amd64"} {
		artifact, exists := release.Artifacts[target]
		if !exists || validateManagedTool(release.spec(artifact)) != nil {
			return false
		}
	}

	return true
}
