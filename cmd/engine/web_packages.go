//go:build !js

package main

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"unicode"
)

var (
	errNoWebPackages  = errors.New("pattern matched no browser main packages")
	errOutsideWebRoot = errors.New("browser package is outside the current module")
	webMajorVersion   = regexp.MustCompile(`/v([2-9]|[1-9][0-9]+)$`)
)

type webPackage struct {
	Name, ImportPath, RelativePath, BinaryName string
	galleryOrder                               int
}

//nolint:tagliatelle // Match go list's external JSON field names exactly.
type packageMetadata struct {
	Name       string `json:"Name"`
	ImportPath string `json:"ImportPath"`
	Dir        string `json:"Dir"`
}

// -find identifies target-specific packages without loading imports. This allows
// discovery before ignored, generated asset packages have been prepared.
func listWebPackages(ctx context.Context, root, pattern string) ([]webPackage, error) {
	//nolint:gosec // The selected Go package is passed as one argument, never shell code.
	command := exec.CommandContext(ctx, "go", "list", "-find", "-json", pattern)
	command.Dir = root

	command.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm", "CGO_ENABLED=0")

	var diagnostics bytes.Buffer

	command.Stderr = &diagnostics

	encoded, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("discover browser packages %s: %w: %s", pattern, err, strings.TrimSpace(diagnostics.String()))
	}

	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return nil, fmt.Errorf("resolve module root: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(encoded))

	var packages []webPackage

	for {
		var item packageMetadata
		if err := decoder.Decode(&item); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}

			return nil, fmt.Errorf("read browser package metadata: %w", err)
		}

		if item.Name != "main" {
			continue
		}

		entry, err := localWebPackage(root, item)
		if err != nil {
			return nil, err
		}

		packages = append(packages, entry)
	}

	if len(packages) == 0 {
		return nil, fmt.Errorf("%w: %s", errNoWebPackages, pattern)
	}

	slices.SortFunc(packages, func(first, second webPackage) int {
		if order := cmp.Compare(first.galleryOrder, second.galleryOrder); order != 0 {
			return order
		}

		return strings.Compare(first.RelativePath, second.RelativePath)
	})

	return packages, nil
}

func localWebPackage(root string, item packageMetadata) (webPackage, error) {
	directory, err := filepath.EvalSymlinks(item.Dir)
	if err != nil {
		return webPackage{}, fmt.Errorf("resolve browser package %s: %w", item.ImportPath, err)
	}

	relative, err := filepath.Rel(root, directory)
	if err != nil || !filepath.IsLocal(relative) {
		return webPackage{}, fmt.Errorf("%w: %s", errOutsideWebRoot, item.ImportPath)
	}

	name := strings.ReplaceAll(strings.ReplaceAll(filepath.Base(directory), "-", " "), "_", " ")

	letters := []rune(name)
	if len(letters) > 0 {
		letters[0] = unicode.ToUpper(letters[0])
	}

	title, order := repositorySamplePresentation(item.ImportPath)
	if title == "" {
		title = string(letters)
	}

	return webPackage{
		Name: title, ImportPath: item.ImportPath, RelativePath: relative,
		BinaryName: webBinaryName(item.ImportPath), galleryOrder: order,
	}, nil
}

// Match go build's last non-major-version path component. go list's Target
// is empty for cross-compilation with GOBIN set (including mise environments).
func webBinaryName(importPath string) string {
	if webMajorVersion.MatchString(importPath) {
		importPath = path.Dir(importPath)
	}

	return path.Base(importPath)
}
