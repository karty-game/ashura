//go:build !js

package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLegacyLightmapCommandRejectsBeforeInputOrExecutor(t *testing.T) {
	t.Parallel()

	// Missing input would cause an IO error if the retired command reached admission.
	input := filepath.Join(t.TempDir(), "missing.klightmap")
	for _, args := range [][]string{
		nil,
		{"--unknown"},
		{"--input", input, "--output", input + ".ktx2"},
		{"--input", input, "--output", input + ".ktx2", "--browser", "--browser-executable", "/must-not-run/browser"},
		{"--input", input, "--output", input + ".ktx2", "--rnm", "--browser", "--browser-executable", "/must-not-run/browser"},
	} {
		command := append([]string{assetCommand, "lightmap"}, args...)
		if err := run(command); !errors.Is(err, errLegacyLightmapCommand) ||
			!strings.Contains(err.Error(), "ProductionBake") || !strings.Contains(err.Error(), "engine assets build") {
			t.Fatal("retired CLI reached input/executor admission", err)
		}
	}
}

func TestRetiredLightmapCommandPreservesExistingOutput(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	input, output := filepath.Join(directory, "inputs.klightmap"), filepath.Join(directory, "baked.ktx2")

	if err := os.WriteFile(input, []byte("invalid"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(output, []byte("preserved"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := run([]string{assetCommand, "lightmap", "--input", input, "--output", output, "--browser"})
	if !errors.Is(err, errLegacyLightmapCommand) {
		t.Fatal("retired command did not return migration guidance", err)
	}
	//nolint:gosec // The output is test-owned and confined to t.TempDir.
	data, err := os.ReadFile(output)
	if err != nil || string(data) != "preserved" {
		t.Fatal("retired command changed existing output", err)
	}
}
