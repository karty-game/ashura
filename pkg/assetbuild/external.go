//go:build !js

package assetbuild

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	toolDependency  = "tool"
	toolUnavailable = "unavailable"
)

var errExternalTool = errors.New("asset processor executable is unavailable or invalid")

// TrackTool resolves an executable and records its content, permission mode and
// resolution as an action dependency. Basenames search PATH; other relative paths
// are relative to the project. Explicit tool paths may be outside the project.
// The returned absolute path can be passed directly to exec.CommandContext.
// Plugins must separately track tool inputs, libraries and relevant environment.
func (ctx *Context) TrackTool(name string) (string, error) {
	if ctx.expanding {
		return "", ctx.discoveryError()
	}

	if !validToolName(name) {
		return "", fmt.Errorf("tool name %q: %w", name, errExternalTool)
	}

	if toolUsesPath(name) {
		_ = ctx.Getenv("PATH")
	}

	if runtime.GOOS == "windows" {
		_ = ctx.Getenv("PATHEXT")
	}

	resolved, identity, err := ctx.runner.toolSnapshot(name)
	ctx.deps = append(ctx.deps, dependency{Kind: toolDependency, Name: name, Value: identity})

	return resolved, err
}

func validToolName(name string) bool {
	return name != "" && !strings.ContainsRune(name, '\x00')
}

func toolUsesPath(name string) bool {
	return !filepath.IsAbs(name) && !strings.ContainsAny(name, "/\\")
}

func (run *runner) toolSnapshot(name string) (string, string, error) {
	if run.snapshotMu != nil {
		run.snapshotMu.Lock()
		defer run.snapshotMu.Unlock()
	}
	// A NUL-prefixed key cannot collide with a project filename. Reuse the existing
	// per-build snapshot store to avoid hashing a large tool once per source asset.
	key := "\x00tool:" + name
	if snapshot, exists := run.files[key]; exists {
		return string(snapshot.data), snapshot.hash, snapshot.err
	}

	resolved, identity, err := fingerprintTool(run.project, name)
	if err != nil {
		identity = toolUnavailable
	}

	run.files[key] = fileValue{data: []byte(resolved), hash: identity, err: err}

	return resolved, identity, err
}

func fingerprintTool(project, name string) (string, string, error) {
	full, err := resolveTool(project, name)
	if err != nil {
		return "", "", err
	}

	resolved, err := filepath.EvalSymlinks(full)
	if err != nil {
		return "", "", fmt.Errorf("resolve tool %q: %w", name, err)
	}

	info, err := os.Stat(resolved)
	if err != nil {
		return "", "", fmt.Errorf("inspect tool %q: %w", name, err)
	}

	data, err := readBounded(resolved)
	if err != nil {
		return "", "", fmt.Errorf("fingerprint tool %q: %w", name, err)
	}

	identity, err := jsonDigest(struct {
		Path string `json:"path"`
		Hash string `json:"hash"`
		Mode uint32 `json:"mode"`
	}{Path: resolved, Hash: digest(data), Mode: uint32(info.Mode().Perm())})
	if err != nil {
		return "", "", err
	}

	return resolved, identity, nil
}

func resolveTool(project, name string) (string, error) {
	if !validToolName(name) {
		return "", fmt.Errorf("tool name %q: %w", name, errExternalTool)
	}

	if !toolUsesPath(name) {
		full := name
		if !filepath.IsAbs(full) {
			full = filepath.Join(project, full)
		}

		return executablePath(full, name)
	}

	for _, directory := range filepath.SplitList(os.Getenv("PATH")) {
		if directory == "" {
			directory = project
		} else if !filepath.IsAbs(directory) {
			directory = filepath.Join(project, directory)
		}

		resolved, err := exec.LookPath(filepath.Join(directory, name))
		if err == nil {
			return resolved, nil
		}
	}

	return "", fmt.Errorf("tool %q was not found on PATH: %w", name, errExternalTool)
}

func executablePath(full, name string) (string, error) {
	resolved, err := exec.LookPath(filepath.Clean(full))
	if err != nil {
		return "", fmt.Errorf("tool %q: %w", name, err)
	}

	return resolved, nil
}
