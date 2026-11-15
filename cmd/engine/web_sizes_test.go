//go:build !js

package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func TestBudgetCountsEveryAssetAndSharedFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	for path, length := range map[string]int64{
		"apps/samples/basic/app.wasm":             1,
		"apps/samples/basic/levels/lightmap.ktx2": defaultSampleLimit - 1,
		"apps/samples/other/app.wasm":             1,
		"wasm_exec.js":                            1,
	} {
		sizedFile(t, root, path, length)
	}

	report, err := inspectWebExport(root, budgetSamplePackages("basic", "other"))
	if err != nil {
		t.Fatal(err)
	}

	if report.total != defaultSampleLimit+2 || report.files != 4 || report.shared != 1 {
		t.Fatal("incomplete accounting", report)
	}

	var output bytes.Buffer
	if err = printReport(&output, report); !errors.Is(err, errBudget) {
		t.Fatal("external assets/shared host must fail budget", err, output.String())
	}
}

func TestSponzaLimitCountsAllPublishedProfiles(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	sizedFile(t, root, "apps/samples/sponza/app.wasm", 1)
	sizedFile(t, root, "apps/samples/sponza/levels/profiles/desktop/payload.ktx2", sponzaSampleLimit-2)
	sizedFile(t, root, "apps/samples/sponza/levels/profiles/mobile/payload.ktx2", 1)
	sizedFile(t, root, "apps/samples/basic/app.wasm", 1)

	packages := budgetSamplePackages("sponza", "basic")

	report, err := inspectWebExport(root, packages)
	if err != nil {
		t.Fatal(err)
	}

	if err = printReport(&bytes.Buffer{}, report); err != nil {
		t.Fatal("exact complete limit rejected", err)
	}

	sizedFile(t, root, "apps/samples/sponza/levels/unreferenced-extra.bin", 1)

	report, err = inspectWebExport(root, packages)
	if err != nil {
		t.Fatal(err)
	}

	if err = printReport(&bytes.Buffer{}, report); !errors.Is(err, errBudget) {
		t.Fatal("unreferenced published file was excluded", err)
	}
}

func TestOnlySponzaGetsExceptionAndMissingAppsFail(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	sizedFile(t, root, "apps/samples/sponza-copy/app.wasm", defaultSampleLimit+1)
	sizedFile(t, root, "apps/samples/basic/app.wasm", 1)

	packages := budgetSamplePackages("sponza-copy", "basic")

	report, err := inspectWebExport(root, packages)
	if err != nil {
		t.Fatal(err)
	}

	if err = printReport(&bytes.Buffer{}, report); !errors.Is(err, errBudget) {
		t.Fatal("exception applied to another sample", err)
	}

	sizedFile(t, root, "apps/samples/missing/asset.bin", 1)

	packages = append(packages, budgetSamplePackages("missing")...)
	if _, err = inspectWebExport(root, packages); !errors.Is(err, errLayout) || !strings.Contains(err.Error(), "missing") {
		t.Fatal("sample without app admitted", err)
	}
}

func TestSingleSampleBudgetCountsWholeFlatExport(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	sizedFile(t, root, wasmFile, defaultSampleLimit-2)
	sizedFile(t, root, "profiles/extra.bin", 1)
	sizedFile(t, root, wasmExecFile, 1)

	packages := budgetSamplePackages("new-sample")
	if err := checkWebSampleBudgets(root, packages, &bytes.Buffer{}); err != nil {
		t.Fatal("exact flat export budget rejected", err)
	}

	sizedFile(t, root, "unreferenced.bin", 1)

	if err := checkWebSampleBudgets(root, packages, &bytes.Buffer{}); !errors.Is(err, errBudget) {
		t.Fatal("flat export excluded a file", err)
	}
}

func TestSampleBudgetsUseCanonicalPackageIdentity(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		item  webPackage
		limit int64
	}{
		{budgetSamplePackages("sponza")[0], sponzaSampleLimit},
		{budgetSamplePackages("future/sample")[0], defaultSampleLimit},
		{budgetSamplePackages("sponza-copy")[0], defaultSampleLimit},
		{webPackage{ImportPath: "example.test/app/samples/sponza", RelativePath: filepath.Join("samples", "sponza")}, 0},
		{webPackage{ImportPath: repositorySamples + "sponza", RelativePath: "elsewhere"}, 0},
		{webPackage{ImportPath: repositorySamples + "../sponza", RelativePath: filepath.Join("samples", "sponza")}, 0},
	} {
		if _, got := repositorySampleBudget(test.item); got != test.limit {
			t.Fatalf("package %+v: limit %d, want %d", test.item, got, test.limit)
		}
	}
}

func TestMixedExportCountsOrphansAndForeignApplications(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	sizedFile(t, root, "apps/samples/new-sample/app.wasm", 1)
	sizedFile(t, root, "apps/sponza/app.wasm", defaultSampleLimit-1)
	sizedFile(t, root, "apps/samples/orphan/payload.bin", 1)

	packages := append(budgetSamplePackages("new-sample"), webPackage{ImportPath: "example.test/sponza", RelativePath: "sponza"})
	if err := checkWebSampleBudgets(root, packages, &bytes.Buffer{}); !errors.Is(err, errBudget) {
		t.Fatal("mixed export excluded foreign/orphan bytes", err)
	}
}

func TestBudgetRejectsSymlinksAndNonregularFiles(t *testing.T) {
	t.Parallel()

	// Portable DirEntry fixtures do not require Windows symlink privileges or
	// an operating system that can create Unix pipes on disk.
	entries, err := (fstest.MapFS{
		"link": &fstest.MapFile{Mode: os.ModeSymlink},
		"pipe": &fstest.MapFile{Mode: os.ModeNamedPipe},
	}).ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()

	var report gallerySize

	for _, entry := range entries {
		if err := report.countFile(root, filepath.Join(root, entry.Name()), entry); !errors.Is(err, errLayout) {
			t.Fatal("published nonregular file admitted", entry.Name(), err)
		}
	}
}

func budgetSamplePackages(names ...string) []webPackage {
	packages := make([]webPackage, len(names))
	for index, name := range names {
		packages[index] = webPackage{ImportPath: repositorySamples + name, RelativePath: filepath.Join("samples", filepath.FromSlash(name))}
	}

	return packages
}

func sizedFile(t *testing.T, root, path string, length int64) {
	t.Helper()

	destination := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
		t.Fatal(err)
	}

	file, err := os.Create(destination) // #nosec G304 -- test-only path assembled beneath t.TempDir from fixed fixture names.
	if err != nil {
		t.Fatal(err)
	}

	if err = file.Truncate(length); err != nil {
		_ = file.Close()

		t.Fatal(err)
	}

	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
}
