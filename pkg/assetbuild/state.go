//go:build !js

package assetbuild

import (
	"fmt"
	"path/filepath"
)

const (
	stateDirectory = "generated/cache"
	cacheDirectory = stateDirectory + "/assets/v1"
	lockName       = stateDirectory + "/asset-build/lock"
	journalName    = stateDirectory + "/asset-build/publication.json"
)

func containsPath(root, full string) bool {
	relative, err := filepath.Rel(root, full)

	return err == nil && filepath.IsLocal(relative)
}

// publicationPath reserves build state even when Output is its parent generated/.
func (run *runner) publicationPath(name string) (string, error) {
	full, err := confined(run.output, name)
	if err != nil {
		return "", err
	}

	if overlaps(full, run.buildState) {
		return "", fmt.Errorf("generated output %q overlaps build state: %w", name, errPath)
	}

	return full, nil
}
