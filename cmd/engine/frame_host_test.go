//go:build !js

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/karty-game/karty-webgpu/internal/platform/browser/framehost"
)

func TestSamplePageEmbedsFrameHostBeforeApplication(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	if err := writeSamplePage(directory, "../wasm_exec.js", "</script><script>hostile</script>"); err != nil {
		t.Fatal(err)
	}

	//nolint:gosec // The fixed filename lives inside this test's private temporary directory.
	data, err := os.ReadFile(filepath.Join(directory, "index.html"))
	if err != nil {
		t.Fatal(err)
	}

	page := string(data)

	hostPosition, startPosition := strings.Index(page, framehost.Source), strings.Index(page, "const go = new Go();")
	if hostPosition < 0 || startPosition <= hostPosition || strings.Contains(page, "__ASHURA_FRAME_HOST__") ||
		strings.Contains(page, "<script>hostile") || !strings.Contains(page, `src="../wasm_exec.js"`) {
		t.Fatal("sample page omitted/reordered its static frame host or lost HTML escaping")
	}

	files, err := os.ReadDir(directory)
	if err != nil || len(files) != 1 || files[0].Name() != "index.html" {
		t.Fatal("inline host created an extra distribution file", files, err)
	}
}

func TestSamplePageDoesNotExpandHostMarkerInTitle(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	if err := writeSamplePage(directory, "../wasm_exec.js", "__ASHURA_FRAME_HOST__"); err != nil {
		t.Fatal(err)
	}

	//nolint:gosec // The fixed filename lives inside this test's private temporary directory.
	data, err := os.ReadFile(filepath.Join(directory, "index.html"))
	if err != nil {
		t.Fatal(err)
	}

	page := string(data)
	if !strings.Contains(page, "<title>__ASHURA_FRAME_HOST__ — Ashura</title>") || strings.Count(page, framehost.Source) != 1 {
		t.Fatal("untrusted title expanded the static host placeholder")
	}
}
