//go:build !js

package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestWebPackageDiscovery(t *testing.T) {
	// This isolated module has no VCS checkout; ordinary preview builds retain stamping.
	t.Setenv("GOFLAGS", strings.TrimSpace(os.Getenv("GOFLAGS")+" -buildvcs=false"))

	root := t.TempDir()
	writeWebFixtures(t, root, map[string]string{
		"go.mod":                                   "module example.test/gallery\n\ngo 1.27.0\n",
		"samples/zeta/main.go":                     "package main\nfunc main() {}\n",
		"samples/alpha/main.go":                    "package main\nimport _ \"example.test/gallery/samples/alpha/generated/missing\"\nfunc main() {}\n",
		"samples/browser/main.go":                  "//go:build js && wasm\n\npackage main\nfunc main() {}\n",
		"samples/native/main.go":                   "//go:build !js\n\npackage main\nfunc main() {}\n",
		"samples/library/library.go":               "package library\n",
		"samples/alpha/generated/assets/assets.go": "package assets\n",
	})

	packages, err := listWebPackages(t.Context(), root, "./samples/...")
	if err != nil {
		t.Fatal(err)
	}

	paths := make([]string, 0, len(packages))

	for _, item := range packages {
		paths = append(paths, filepath.ToSlash(item.RelativePath))
		if item.ImportPath != "example.test/gallery/"+filepath.ToSlash(item.RelativePath) {
			t.Fatalf("package identity mismatch: %+v", item)
		}

		if item.BinaryName != filepath.Base(item.RelativePath) {
			t.Fatalf("package output name mismatch: %+v", item)
		}
	}

	want := []string{"samples/alpha", "samples/browser", "samples/zeta"}
	if !slices.Equal(paths, want) {
		t.Fatalf("web packages = %v, want %v", paths, want)
	}

	for _, pattern := range []string{"./samples/library", "./samples/missing"} {
		if _, err := listWebPackages(t.Context(), root, pattern); err == nil {
			t.Errorf("accepted pattern without runnable packages: %s", pattern)
		}
	}
}

func writeWebFixtures(t *testing.T, root string, files map[string]string) {
	t.Helper()

	for name, contents := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestWebBinaryNames(t *testing.T) {
	t.Parallel()

	for importPath, want := range map[string]string{
		"example.test/view":           "view",
		"example.test/view/v2":        "view",
		"example.test/view/v123":      "view",
		"example.test/view/v1":        "v1",
		"example.test/view/v01":       "v01",
		"example.test/view/v2/nested": "nested",
	} {
		if got := webBinaryName(importPath); got != want {
			t.Errorf("output name for %s: got %s, want %s", importPath, got, want)
		}
	}
}
