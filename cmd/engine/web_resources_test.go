//go:build !js

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWebResourcesStageOnlyPreparedBytes(t *testing.T) {
	t.Parallel()
	project := t.TempDir()
	destination := t.TempDir()

	source := filepath.Join(project, "generated", "public", "models")
	if err := os.MkdirAll(source, 0o750); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(source, "house.kmodel"), []byte("prepared"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := stageWebResources(project, destination); err != nil {
		t.Fatal(err)
	}

	//nolint:gosec // G304: destination is a private t.TempDir and the payload path is fixed.
	got, err := os.ReadFile(filepath.Join(destination, "models", "house.kmodel"))
	if err != nil || string(got) != "prepared" {
		t.Fatal(err, string(got))
	}

	if err := os.WriteFile(filepath.Join(source, "bad.scene.yaml"), []byte("source"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := stageWebResources(project, t.TempDir()); err == nil {
		t.Fatal("source scene was staged")
	}

	if err := os.Remove(filepath.Join(source, "bad.scene.yaml")); err != nil {
		t.Fatal(err)
	}

	if err := os.Symlink(filepath.Join(source, "house.kmodel"), filepath.Join(source, "alias.kmodel")); err != nil {
		t.Fatal(err)
	}

	if err := stageWebResources(project, t.TempDir()); err == nil {
		t.Fatal("symlink was staged")
	}
}

func TestWebResourcesRejectParentSymlinkAndReservedNames(t *testing.T) {
	t.Parallel()
	project := t.TempDir()

	elsewhere := t.TempDir()
	if err := os.Mkdir(filepath.Join(elsewhere, "public"), 0o750); err != nil {
		t.Fatal(err)
	}

	if err := os.Symlink(elsewhere, filepath.Join(project, "generated")); err != nil {
		t.Fatal(err)
	}

	if err := stageWebResources(project, t.TempDir()); err == nil {
		t.Fatal("followed generated parent symlink")
	}

	for _, name := range []string{"index.html", "app.wasm", "wasm_exec.js", "build-info.json", "INDEX.HTML", "Wasm_Exec.js", "Build-Info.json"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()

			source := filepath.Join(root, "generated", "public")
			if err := os.MkdirAll(source, 0o750); err != nil {
				t.Fatal(err)
			}

			if err := os.WriteFile(filepath.Join(source, name), []byte("collision"), 0o600); err != nil {
				t.Fatal(err)
			}

			if err := stageWebResources(root, t.TempDir()); err == nil {
				t.Fatal("accepted reserved web filename")
			}
		})
	}
}
