//go:build !js

package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

var errWebResource = errors.New("public resource is not a confined prepared payload")

// stageWebResources copies only published prepared payloads into the immutable
// web distribution. Source scenes and Go files never become public resources.
func stageWebResources(project, destination string) error {
	source := project
	for _, part := range []string{"generated", "public"} {
		source = filepath.Join(source, part)

		entry, err := os.Lstat(source)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}

		if err != nil {
			return err
		}

		if !entry.IsDir() || entry.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: %s", errWebResource, source)
		}
	}

	output, err := os.OpenRoot(destination)
	if err != nil {
		return err
	}
	defer func() { _ = output.Close() }()

	return filepath.WalkDir(source, func(name string, entry fs.DirEntry, walkErr error) error {
		return copyWebResource(source, output, name, entry, walkErr)
	})
}

func copyWebResource(source string, output *os.Root, name string, entry fs.DirEntry, walkErr error) error {
	if walkErr != nil {
		return walkErr
	}

	if entry.Type()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: symlink %s", errWebResource, name)
	}

	if entry.IsDir() {
		return nil
	}

	if !entry.Type().IsRegular() || strings.HasSuffix(name, ".go") || strings.HasSuffix(name, ".scene.yaml") {
		return fmt.Errorf("%w: %s", errWebResource, name)
	}

	relative, err := filepath.Rel(source, name)
	if err != nil {
		return err
	}

	if !fs.ValidPath(filepath.ToSlash(relative)) {
		return fmt.Errorf("%w: %s", errWebResource, relative)
	}

	// Reserve compiler-owned files on case-insensitive developer filesystems too.
	switch strings.ToLower(relative) {
	case "index.html", wasmFile, wasmExecFile, "build-info.json":
		return fmt.Errorf("%w: reserved web filename %s", errWebResource, relative)
	}

	if err := output.MkdirAll(filepath.Dir(relative), 0o750); err != nil {
		return err
	}
	//nolint:gosec // G304: WalkDir enumerates the fixed generated/public tree and rejects symlinks above and within it.
	data, err := os.ReadFile(name)
	if err != nil {
		return err
	}

	return output.WriteFile(relative, data, 0o600)
}
