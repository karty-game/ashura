//go:build !js

package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestHostRecipePathIgnoresDuplicateToolPrefixes(t *testing.T) {
	separator := string(os.PathListSeparator)
	base := []string{"/go/bin", "/lint/bin", "/usr/bin", "", "relative-tools"}
	t.Setenv("PATH", strings.Join(base, separator))

	before := hostBuildEnvironment()

	t.Setenv("PATH", strings.Join(append([]string{"/go/bin", "/go/bin", "/lint/bin"}, base...), separator))

	after := hostBuildEnvironment()

	if !slices.Equal(before, after) {
		t.Fatal("duplicate tool prefixes changed recipe environment")
	}

	want := strings.Join([]string{"/lint/bin", "/go/bin", "/usr/bin", "", "relative-tools"}, separator)
	if got := uniqueSearchPath(want); got != want {
		t.Fatalf("changed tool search order: %q versus %q", got, want)
	}
}

func TestRecipeCacheTracksImportedCode(t *testing.T) {
	t.Parallel()
	project := assetRecipeFixture(t)
	recipe := strings.Replace(recipeFixture,
		`import "github.com/karty-game/karty-webgpu/pkg/assetbuild"`,
		`import ("github.com/karty-game/karty-webgpu/pkg/assetbuild"; "example.test/assets/settings")`, 1)
	recipe = strings.Replace(recipe, "size := 2", "size := settings.Size", 1)
	writeWebFixtures(t, project, map[string]string{
		assetConfigFile: recipe, "settings/options.go": "package settings\nconst Size = 2\n",
	})

	var output bytes.Buffer

	options := assetOptions{target: "js/wasm", log: &output}
	for _, cached := range []bool{false, true} {
		if err := prepareProjectAssets(t.Context(), project, options); err != nil {
			t.Fatal(err)
		}

		assertRecipeExecution(t, &output, cached)
	}

	before := readRecipeOutputs(t, project)
	writeWebFixtures(t, project, map[string]string{"settings/options.go": "package settings\nconst Size = 1\n"})

	if err := prepareProjectAssets(t.Context(), project, options); err != nil {
		t.Fatal(err)
	}

	assertRecipeExecution(t, &output, false)

	after := readRecipeOutputs(t, project)
	changed := false

	for name, data := range before {
		changed = changed || !bytes.Equal(data, after[name])
	}

	if !changed {
		t.Fatal("imported recipe change did not update generated assets")
	}
}

func TestRecipeCancellationStopsRunningChild(t *testing.T) {
	t.Parallel()
	project := assetRecipeFixture(t)
	writeWebFixtures(t, project, map[string]string{assetConfigFile: `//go:build ignore
package main
import ("os"; "time")
func main() {
    if err := os.WriteFile("started", nil, 0600); err != nil { panic(err) }
    time.Sleep(time.Minute)
}
`})

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	finished := make(chan error, 1)

	go func() {
		var output bytes.Buffer
		finished <- prepareProjectAssets(ctx, project, assetOptions{log: &output})
	}()

	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()

	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()

	for {
		if _, err := os.Stat(filepath.Join(project, "started")); err == nil {
			break
		}

		select {
		case err := <-finished:
			t.Fatalf("recipe exited before cancellation: %v", err)
		case <-deadline.C:
			t.Fatal("recipe did not start")
		case <-tick.C:
		}
	}

	cancel()

	select {
	case err := <-finished:
		if err == nil {
			t.Fatal("canceled recipe succeeded")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("recipe child survived cancellation and kept its output pipe open")
	}
}
