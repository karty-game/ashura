//go:build !js

package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestWebGalleryBuildWithDuplicateNames(t *testing.T) {
	t.Setenv("GOFLAGS", "-buildvcs=false")
	t.Setenv("GOBIN", t.TempDir())
	root := t.TempDir()
	writeWebFixtures(t, root, map[string]string{
		"go.mod":          "module example.test/gallery\n\ngo 1.27.0\n",
		"a/view/main.go":  "package main\nfunc main() { println(\"first sample\") }\n",
		"b/view/main.go":  "package main\nfunc main() { println(\"second sample\") }\n",
		"c/other/main.go": "package main\nfunc main() { println(\"third sample\") }\n",
	})

	dist := t.TempDir()
	if err := buildWebPreview(t.Context(), root, dist, "./...", webOptions{}); err != nil {
		t.Fatal(err)
	}

	var previous []byte

	for _, name := range []string{"a/view", "b/view", "c/other"} {
		//nolint:gosec // These fixed sample names are beneath the test's private distribution.
		binary, err := os.ReadFile(filepath.Join(dist, "apps", name, wasmFile))
		if err != nil || !bytes.HasPrefix(binary, []byte("\x00asm")) {
			t.Fatalf("invalid sample %s: %v", name, err)
		}

		if bytes.Equal(previous, binary) {
			t.Fatal("distinct samples share an executable")
		}

		previous = binary
		//nolint:gosec // These fixed sample names are beneath the test's private distribution.
		page, err := os.ReadFile(filepath.Join(dist, "apps", name, "index.html"))
		if err != nil || !strings.Contains(string(page), "../../../wasm_exec.js") {
			t.Fatalf("sample runtime URL %s: %s %v", name, page, err)
		}
	}

	if _, err := os.Stat(filepath.Join(dist, "index.html")); err != nil {
		t.Fatal(err)
	}

	writeWebFixtures(t, root, map[string]string{"b/view/main.go": "invalid Go source"})
	failed := t.TempDir()

	if err := buildWebPreview(t.Context(), root, failed, "./...", webOptions{}); err == nil {
		t.Fatal("gallery accepted a failed sample")
	}

	if _, err := os.Stat(filepath.Join(failed, "index.html")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("published an incomplete gallery")
	}
}

func TestWebAssetsPrepareConcurrently(t *testing.T) {
	t.Parallel()

	if runtime.GOMAXPROCS(0) < 2 {
		t.Skip("requires at least two project workers")
	}

	root := assetRecipeFixture(t)
	// Each real recipe waits for its peer. Serial preparation cannot finish.
	recipe := `//go:build ignore
package main
import ("fmt"; "os"; "path/filepath"; "time")
func main() {
    here, _ := os.Getwd()
    self := filepath.Base(here)
    peer := "first"
    if self == peer { peer = "second" }
    if err := os.WriteFile("ready", nil, 0600); err != nil { panic(err) }
    deadline := time.Now().Add(10*time.Second)
    for {
        if _, err := os.Stat(filepath.Join("..", peer, "ready")); err == nil { break }
        if time.Now().After(deadline) { panic("peer recipe did not start concurrently") }
        time.Sleep(10*time.Millisecond)
    }
    fmt.Println("finished " + self)
}
`
	writeWebFixtures(t, root, map[string]string{
		"first/asset.config.go": recipe, "second/asset.config.go": recipe,
	})

	packages := []webPackage{
		{ImportPath: "example.test/assets/first", RelativePath: "first"},
		{ImportPath: "example.test/assets/second", RelativePath: "second"},
	}

	var output bytes.Buffer

	if err := prepareWebAssets(t.Context(), root, packages, assetOptions{log: &output}); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"first", "second"} {
		if !strings.Contains(output.String(), "finished "+name) {
			t.Fatalf("missing recipe output: %s", &output)
		}
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if err := prepareWebAssets(ctx, root, packages, assetOptions{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("lost cancellation: %v", err)
	}
}
