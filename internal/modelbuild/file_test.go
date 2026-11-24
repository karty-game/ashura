//go:build !js

package modelbuild_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/karty-game/ashura/internal/modelbuild"
	"github.com/karty-game/ashura/internal/modelbuild/testfixture"
)

func TestFilePreparationIsAtomicDeterministicAndConfined(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	source := filepath.Join(directory, "source.glb")
	output := filepath.Join(directory, "generated", "source.kmodel")

	document, data := testfixture.Triangle()
	if err := os.WriteFile(source, testfixture.GLB(document, data), 0o600); err != nil {
		t.Fatal(err)
	}

	first, err := modelbuild.Prepare(source, output)
	if err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(output)
	if err != nil {
		t.Fatal(err)
	}

	second, err := modelbuild.Prepare(source, output)
	if err != nil {
		t.Fatal(err)
	}

	after, err := os.Stat(output)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(first.Data, second.Data) || !info.ModTime().Equal(after.ModTime()) {
		t.Fatal("unchanged output was rewritten or differs")
	}

	if _, err := modelbuild.Prepare(source, source); err == nil {
		t.Fatal("source/output overlap accepted")
	}

	document["animations"] = []any{map[string]any{}}
	if err := os.WriteFile(source, testfixture.GLB(document, data), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := modelbuild.Prepare(source, output); err == nil {
		t.Fatal("unsupported source accepted")
	}

	//nolint:gosec // Test-owned path beneath t.TempDir.
	preserved, err := os.ReadFile(output)
	if err != nil || !bytes.Equal(preserved, first.Data) {
		t.Fatal("failed conversion replaced prior valid output")
	}
}

func TestSourceAndOutputSymlinksRejected(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	source := filepath.Join(directory, "source.glb")

	document, data := testfixture.Triangle()
	if err := os.WriteFile(source, testfixture.GLB(document, data), 0o600); err != nil {
		t.Fatal(err)
	}

	linked := filepath.Join(directory, "linked.glb")
	if err := os.Symlink(source, linked); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if _, err := modelbuild.Prepare(linked, filepath.Join(directory, "prepared.kmodel")); err == nil {
		t.Fatal("source symlink accepted")
	}

	linkedParent := filepath.Join(directory, "redirect")
	if err := os.Symlink(directory, linkedParent); err != nil {
		t.Fatal(err)
	}

	if _, err := modelbuild.Prepare(source, filepath.Join(linkedParent, "prepared.kmodel")); err == nil {
		t.Fatal("output parent symlink accepted")
	}
}

func TestHugeSparsePreviousOutputIsNotReadIntoMemory(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	source, output := filepath.Join(directory, "source.glb"), filepath.Join(directory, "previous.kmodel")

	document, data := testfixture.Triangle()
	if err := os.WriteFile(source, testfixture.GLB(document, data), 0o600); err != nil {
		t.Fatal(err)
	}

	//nolint:gosec // Test-owned sparse output beneath t.TempDir.
	file, err := os.Create(output)
	if err != nil {
		t.Fatal(err)
	}

	if err := file.Truncate(4 << 30); err != nil {
		_ = file.Close()

		t.Skipf("sparse file unavailable: %v", err)
	}

	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	result, err := modelbuild.Prepare(source, output)
	if err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(output)
	if err != nil || info.Size() != int64(len(result.Data)) {
		t.Fatal("sparse output was not safely replaced")
	}
}
