//go:build !js

package main

import (
	"bytes"
	"errors"
	"os/exec"
	"strings"
	"testing"
)

func TestAssetRecipeStderrRetainsBoundedTail(t *testing.T) {
	t.Parallel()

	var diagnostics assetRecipeStderr

	var written strings.Builder

	for _, chunk := range []string{
		"", "prefix", strings.Repeat("a", assetRecipeStderrLimit-1), "boundary",
		strings.Repeat("b", 2*assetRecipeStderrLimit), "\nfinal bake failure\n",
	} {
		count, err := diagnostics.Write([]byte(chunk))
		if count != len(chunk) || err != nil {
			t.Fatal("stderr was not fully drained", count, len(chunk), err)
		}

		_, _ = written.WriteString(chunk)

		want := written.String()
		if len(want) > assetRecipeStderrLimit {
			want = want[len(want)-assetRecipeStderrLimit:]
		}

		if got := diagnostics.String(); got != want {
			t.Fatalf("stderr tail differs: retained %d bytes, want %d", len(got), len(want))
		}
	}
}

func TestGalleryRecipeFailureRetainsStderrCause(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeWebFixtures(t, root, map[string]string{
		"go.mod": "module example.test/recipe-diagnostics\n\ngo 1.27.0\n",
		"sample/asset.config.go": `//go:build ignore
package main
import ("fmt"; "os")
func main() {
    fmt.Fprintln(os.Stdout, "surface-support HIT (verified inputs and outputs)")
    fmt.Fprintln(os.Stderr, "sample bake failed: selected adapter does not support compute")
    os.Exit(7)
}
`,
	})

	packages := []webPackage{{ImportPath: "example.test/recipe-diagnostics/sample", RelativePath: "sample"}}

	var output bytes.Buffer

	err := prepareWebAssets(t.Context(), root, packages, assetOptions{log: &output})
	if _, ok := errors.AsType[*exec.ExitError](err); !ok {
		t.Fatalf("lost subprocess failure: %v", err)
	}

	if !strings.Contains(output.String(), "surface-support HIT") ||
		!strings.Contains(err.Error(), "sample bake failed: selected adapter does not support compute") ||
		!strings.Contains(err.Error(), packages[0].ImportPath) {
		t.Fatalf("final gallery failure lost its project or cause after buffered progress: %s\n%v", &output, err)
	}
}
