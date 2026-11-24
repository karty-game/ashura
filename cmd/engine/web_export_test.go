//go:build !js

package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseWebExportArgs(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		args    []string
		pattern string
		output  string
	}{
		{nil, defaultPackage, "dist/samples"},
		{[]string{"./samples/...", "--output", "dist/gallery"}, "./samples/...", "dist/gallery"},
		{[]string{"--output=dist/gallery", "./samples/..."}, "./samples/...", "dist/gallery"},
	} {
		pattern, output, err := parseWebExportArgs(test.args)
		if err != nil || pattern != test.pattern || output != test.output {
			t.Errorf("parse %v: got %q, %q, %v", test.args, pattern, output, err)
		}
	}

	for _, args := range [][]string{
		{"--output"}, {"--output="}, {"--output", "--addr"},
		{"--addr", "127.0.0.1:9000"}, {"./one", "./two"},
	} {
		if _, _, err := parseWebExportArgs(args); !errors.Is(err, errUsage) {
			t.Errorf("accepted invalid export arguments %v: %v", args, err)
		}
	}
}

func TestWebExportRejectsBudgetBeforePublishing(t *testing.T) {
	t.Setenv("GOFLAGS", strings.TrimSpace(os.Getenv("GOFLAGS")+" -buildvcs=false"))
	root := t.TempDir()
	writeWebFixtures(t, root, map[string]string{
		"go.mod":                  "module github.com/karty-game/ashura\n\ngo 1.27.0\n",
		"samples/new/main.go":     "package main\nfunc main() {}\n",
		"samples/another/main.go": "package main\nfunc main() {}\n",
	})
	// This prepared orphan alone fills the complete limit; compiled WASM and
	// host files must make both single and partial-gallery exports fail.
	sizedFile(t, root, "samples/new/generated/public/profiles/unreferenced.bin", defaultSampleLimit)
	parent := t.TempDir()
	writeWebFixtures(t, parent, map[string]string{"existing/site.txt": "preserve previous publication"})

	for _, pattern := range []string{"./samples/new", "./samples/..."} {
		var log bytes.Buffer

		output := filepath.Join(parent, "rejected")

		options := webOptions{assets: assetOptions{log: &log}}
		if err := buildWebDistribution(t.Context(), root, output, pattern, options); !errors.Is(err, errBudget) {
			t.Fatalf("%s: oversized export returned %v", pattern, err)
		}

		if !strings.Contains(log.String(), "limit=20000000") || !strings.Contains(log.String(), "assets=") {
			t.Fatalf("size report missing before rejection: %s", &log)
		}

		entries, err := os.ReadDir(parent)
		if err != nil || len(entries) != 1 || entries[0].Name() != "existing" {
			t.Fatalf("budget failure published output or retained staging: %v, %v", entries, err)
		}
	}

	//nolint:gosec // Fixed test fixture under a private temporary directory.
	previous, err := os.ReadFile(filepath.Join(parent, "existing", "site.txt"))
	if err != nil || string(previous) != "preserve previous publication" {
		t.Fatal("budget failure changed an existing publication", err)
	}
}

func TestWebExportPublishesCompleteGallery(t *testing.T) {
	// Only this isolated module disables VCS stamping.
	t.Setenv("GOFLAGS", strings.TrimSpace(os.Getenv("GOFLAGS")+" -buildvcs=false"))

	root := t.TempDir()
	writeWebFixtures(t, root, map[string]string{
		"go.mod":                             "module example.test/export\n\ngo 1.27.0\n",
		"samples/one/main.go":                "package main\nfunc main() {}\n",
		"samples/two/main.go":                "package main\nfunc main() {}\n",
		"samples/two/generated/public/a.bin": "prepared resource",
	})

	output := filepath.Join(t.TempDir(), "site")
	if err := buildWebDistribution(t.Context(), root, output, "./samples/...", webOptions{}); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"index.html", wasmExecFile, "apps/samples/one/index.html", "apps/samples/two/index.html"} {
		if _, err := os.Stat(filepath.Join(output, name)); err != nil {
			t.Fatal(err)
		}
	}

	for _, sample := range []string{"one", "two"} {
		//nolint:gosec // Read fixed sample paths inside this test's temporary export.
		data, err := os.ReadFile(filepath.Join(output, "apps", "samples", sample, wasmFile))
		if err != nil || len(data) < 8 || string(data[:4]) != "\x00asm" {
			t.Fatalf("%s is not a compiled WASM module: %v", sample, err)
		}
	}

	//nolint:gosec // Read a fixed prepared resource inside this test's temporary export.
	data, err := os.ReadFile(filepath.Join(output, "apps/samples/two/a.bin"))
	if err != nil || string(data) != "prepared resource" {
		t.Fatalf("prepared resource not exported: %q, %v", data, err)
	}

	if err := buildWebDistribution(t.Context(), root, output, "./samples/...", webOptions{}); !errors.Is(err, os.ErrExist) {
		t.Fatalf("existing output was not refused: %v", err)
	}

	failed := filepath.Join(filepath.Dir(output), "failed")
	if err := buildWebDistribution(t.Context(), root, failed, "./missing", webOptions{}); err == nil {
		t.Fatal("export accepted a missing package")
	}

	entries, err := os.ReadDir(filepath.Dir(output))
	if err != nil || len(entries) != 1 || entries[0].Name() != "site" {
		t.Fatalf("failed export left partial output or staging: %v, %v", entries, err)
	}
}
