//go:build !js

package assetbuild

import (
	"fmt"
	"os"
	"path/filepath"
)

const libraryDependency = "library"

// TrackLibrary fingerprints a native library without requiring executable
// permission. The caller supplies an absolute path selected by its binding.
// This explicit external dependency never relaxes project source confinement.
func (ctx *Context) TrackLibrary(path string) (string, error) {
	if ctx.expanding {
		return "", ctx.discoveryError()
	}

	if !filepath.IsAbs(path) || !validToolName(path) {
		return "", fmt.Errorf("native library requires an absolute path: %w", errExternalTool)
	}

	resolved, identity, err := ctx.runner.librarySnapshot(path)
	ctx.deps = append(ctx.deps, dependency{Kind: libraryDependency, Name: path, Value: identity})

	return resolved, err
}
func (run *runner) librarySnapshot(path string) (string, string, error) {
	if run.snapshotMu != nil {
		run.snapshotMu.Lock()
		defer run.snapshotMu.Unlock()
	}

	key := "\x00library:" + path
	if snapshot, exists := run.files[key]; exists {
		return string(snapshot.data), snapshot.hash, snapshot.err
	}

	resolved, identity, err := fingerprintLibrary(path)
	if err != nil {
		identity = toolUnavailable
	}

	run.files[key] = fileValue{data: []byte(resolved), hash: identity, err: err}

	return resolved, identity, err
}
func fingerprintLibrary(path string) (string, string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", "", err
	}

	info, err := os.Stat(resolved)
	if err != nil {
		return "", "", err
	}

	if !info.Mode().IsRegular() {
		return "", "", fmt.Errorf("native library is not a regular file: %w", errExternalTool)
	}

	data, err := readBounded(resolved)
	if err != nil {
		return "", "", err
	}

	identity, err := jsonDigest(struct{ Path, Hash string }{resolved, digest(data)})

	return resolved, identity, err
}
