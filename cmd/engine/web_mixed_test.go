//go:build !js

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAutomaticSampleCompilerUsesCanonicalIdentity(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		item      webPackage
		requested string
		want      string
	}{
		{budgetSamplePackages("basic-scene")[0], "", "tinygo"},
		{budgetSamplePackages("basic-scene")[0], "auto", "tinygo"},
		{budgetSamplePackages("basic-scene")[0], "go", "go"},
		{budgetSamplePackages("instancing")[0], "auto", "go"},
		{budgetSamplePackages("sponza")[0], "auto", "go"},
		{budgetSamplePackages("basic-scene-copy")[0], "auto", "go"},
		{webPackage{ImportPath: "example.test/samples/basic-scene", RelativePath: "samples/basic-scene"}, "auto", "go"},
		{webPackage{ImportPath: repositorySamples + "basic-scene", RelativePath: "elsewhere"}, "auto", "go"},
	} {
		if got := sampleWebCompiler(test.item, test.requested); got != test.want {
			t.Fatalf("%+v compiler %q: got %q, want %q", test.item, test.requested, got, test.want)
		}
	}
}

func TestMixedWebGalleryPairsAndExecutesBothRuntimes(t *testing.T) {
	t.Setenv("GOFLAGS", "-buildvcs=false")

	if _, err := exec.LookPath("tinygo"); err != nil {
		t.Skip("mise install supplies pinned TinyGo for Basic Scene")
	}

	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	writeWebFixtures(t, root, map[string]string{
		"go.mod":                                       "module github.com/karty-game/karty-webgpu\n\ngo 1.27.0\n",
		"samples/basic-scene/main.go":                  tinyGoAsyncFixture,
		"samples/instancing/main.go":                   tinyGoAsyncFixture,
		"samples/instancing/generated/public/data.bin": "all prepared files count",
	})
	parent := t.TempDir()

	output := filepath.Join(parent, "gallery")
	if err := buildWebDistribution(t.Context(), root, output, "./samples/...", webOptions{}); err != nil {
		t.Fatal(err)
	}

	receipt := readMixedReceipt[webDistributionReceipt](t, filepath.Join(output, "build-info.json"))
	if receipt.FormatVersion != 1 || receipt.Compiler != "mixed" || len(receipt.Applications) != 2 {
		t.Fatal("missing mixed gallery identities", receipt)
	}

	if _, err := os.Stat(filepath.Join(output, wasmExecFile)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("mixed gallery exposed an ambiguous root bridge", err)
	}

	for _, app := range receipt.Applications {
		assertMixedWebApplication(t, output, app)
	}

	writeWebFixtures(t, root, map[string]string{"samples/instancing/main.go": "package main\nfunc main() { missing() }\n"})

	if err := buildWebDistribution(t.Context(), root, filepath.Join(parent, "failed"), "./samples/...", webOptions{}); err == nil {
		t.Fatal("published mixed gallery with a failed Go app")
	}

	entries, err := os.ReadDir(parent)
	if err != nil || len(entries) != 1 || entries[0].Name() != "gallery" {
		t.Fatal("failed mixed export retained partial output", entries, err)
	}
}

func readMixedReceipt[T any](t *testing.T, path string) T {
	t.Helper()
	//nolint:gosec // Callers supply receipt files within a private fixture export.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var value T
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}

	return value
}

func assertMixedWebApplication(t *testing.T, root string, app webApplicationReceipt) {
	t.Helper()

	directory := filepath.Join(root, filepath.FromSlash(app.Path))

	identity := readMixedReceipt[webApplicationReceipt](t, filepath.Join(directory, "build-info.json"))
	if identity.Path != app.Path || identity.Name != app.Name || identity.RuntimeSHA256 != app.RuntimeSHA256 ||
		identity.RuntimePath != app.RuntimePath {
		t.Fatal("application and gallery compiler identities differ", app, identity)
	}

	want := "go"
	if app.Path == "apps/samples/basic-scene" {
		want = tinyGoCompiler
	}

	if app.Name != want {
		t.Fatal("wrong compiler for sample", app)
	}

	bridgePath := filepath.Join(directory, filepath.FromSlash(app.RuntimePath))
	//nolint:gosec // RuntimePath was produced by the exporter beneath the fixture root.
	bridge, err := os.ReadFile(bridgePath)
	if err != nil {
		t.Fatal(err)
	}

	digest := sha256.Sum256(bridge)
	if hex.EncodeToString(digest[:]) != app.RuntimeSHA256 {
		t.Fatal("exported runtime does not match app receipt")
	}

	//nolint:gosec // Fixed page under the fixture's private app directory.
	page, err := os.ReadFile(filepath.Join(directory, "index.html"))
	if err != nil || !strings.Contains(string(page), `src="`+app.RuntimePath+`"`) {
		t.Fatal("application points at the wrong compiler bridge", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	const run = `const fs=require('node:fs');globalThis.fs=fs;globalThis.crypto=require('node:crypto').webcrypto;
require(process.argv[1]);const go=new Go();
WebAssembly.instantiate(fs.readFileSync(process.argv[2]),go.importObject).then(({instance})=>go.run(instance)).catch(e=>{console.error(e);process.exitCode=1});`
	//nolint:gosec // Staged bridge and module are separate argv, with no shell interpolation.
	result, err := exec.CommandContext(ctx, "node", "-e", run, bridgePath, filepath.Join(directory, wasmFile)).CombinedOutput()
	if err != nil || !bytes.Contains(result, []byte("async bridge ready")) {
		t.Fatalf("%s staged runtime execution: %s %v", app.Name, result, err)
	}
}
