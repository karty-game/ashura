//go:build !js

package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestAssetCatalogJobsReachHostRecipe(t *testing.T) {
	t.Parallel()

	project := t.TempDir()
	writeWebFixtures(t, project, map[string]string{
		"go.mod": "module example.test/job-routing\n\ngo 1.27.3\n",
		assetConfigFile: `package main
import ("os"; "strings")
func main() {
 if err := os.WriteFile("received-arguments.txt", []byte(strings.Join(os.Args[1:], "\n")), 0600); err != nil { panic(err) }
}
`,
	})

	if err := buildAssetCatalog(t.Context(), []string{
		"--project", project, "--target", "js/wasm", "--profile", "release", "--no-cache", "--jobs", "2",
	}); err != nil {
		t.Fatal(err)
	}

	//nolint:gosec // G304: fixed fixture name inside this test's private TempDir.
	data, err := os.ReadFile(filepath.Join(project, "received-arguments.txt"))
	if err != nil {
		t.Fatal(err)
	}

	arguments := strings.Split(string(data), "\n")
	for _, expected := range []string{"--jobs=2", "--target=js/wasm", "--profile=release", "--no-cache"} {
		if !slices.Contains(arguments, expected) {
			t.Fatalf("recipe did not receive %q: %v", expected, arguments)
		}
	}
}

func TestAssetCatalogRejectsInvalidJobsBeforeProcessing(t *testing.T) {
	t.Parallel()

	for _, value := range []string{"-1", "many", ""} {
		if err := buildAssetCatalog(t.Context(), []string{"--jobs=" + value}); err == nil {
			t.Fatalf("accepted invalid build job count %q", value)
		}
	}

	if _, err := normalizeAssetOptions(assetOptions{jobs: -1}); err == nil {
		t.Fatal("negative job count reached project preparation")
	}
}
