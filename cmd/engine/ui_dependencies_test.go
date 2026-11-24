//go:build !js

package main

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Import reachability matters even when runtime guards skip optional work.
// Check the actual sample graph on both targets, including its shared helpers.
func TestBasicSceneDoesNotImportOptionalUI(t *testing.T) {
	t.Parallel()

	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}

	for _, target := range []string{"js/wasm", runtime.GOOS + "/" + runtime.GOARCH} {
		t.Run(target, func(t *testing.T) {
			t.Parallel()

			operatingSystem, architecture, _ := strings.Cut(target, "/")
			command := exec.CommandContext(t.Context(), "go", "list", "-deps", "./samples/basic-scene")
			command.Dir = root

			command.Env = append(webBuildEnvironment("go"), "GOOS="+operatingSystem, "GOARCH="+architecture)

			output, err := command.CombinedOutput()
			if err != nil {
				t.Fatalf("sample dependency graph: %s: %v", output, err)
			}

			for dependency := range strings.FieldsSeq(string(output)) {
				if strings.HasPrefix(dependency, "github.com/go-text/typesetting") ||
					strings.HasPrefix(dependency, "github.com/karty-game/ashura/pkg/ui") ||
					dependency == "github.com/karty-game/ashura/pkg/debug" ||
					dependency == "github.com/karty-game/ashura/samples/internal/sampleui" {
					t.Fatalf("%s basic scene retains optional dependency %s", target, dependency)
				}
			}
		})
	}
}
