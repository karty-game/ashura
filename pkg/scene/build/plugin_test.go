//go:build !js

package scenebuild_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/karty-game/karty-webgpu/pkg/assetbuild"
	scenebuild "github.com/karty-game/karty-webgpu/pkg/scene/build"
	logicbuild "github.com/karty-game/karty-webgpu/pkg/scene/build/logic"
)

//nolint:gosec // G304: every fixture file is under its private t.TempDir.
func TestRecipeCachesReferencedPrefabsAndPreservesOutputsOnFailure(t *testing.T) {
	t.Parallel()
	project := t.TempDir()
	write := func(name, value string) {
		t.Helper()

		if err := os.WriteFile(filepath.Join(project, name), []byte(value), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(
		"root.scene.yaml",
		"version: 1\nroot: {id: root, children: [{id: a, instance: child.scene.yaml}, {id: b, instance: child.scene.yaml}]}",
	)
	write("child.scene.yaml", "version: 1\nroot: {id: child, components: {logic.bool: {initial: false}}}")

	config := assetbuild.Config{Output: "generated",
		Pipelines: []assetbuild.Pipeline{{Name: "fixture",
			Input: []string{"*.scene.yaml"},
			Import: []assetbuild.Plugin{
				scenebuild.Scenes(
					scenebuild.Options{Input: []string{"root.scene.yaml", "child.scene.yaml"}, Modules: logicbuild.Modules()},
				),
			},
		}},
	}
	options := assetbuild.Options{Project: project, Identity: "scene-fixture", Log: io.Discard}

	first, err := assetbuild.Build(context.Background(), config, options)
	if err != nil || first.Executed != 3 {
		t.Fatalf("cold report %+v: %v", first, err)
	}

	output := filepath.Join(project, "generated", "scenes", "Root_gen.go")

	before, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}

	stat, err := os.Stat(output)
	if err != nil {
		t.Fatal(err)
	}

	warm, err := assetbuild.Build(context.Background(), config, options)
	if err != nil || warm.Executed != 0 || warm.Written != 0 {
		t.Fatalf("warm report %+v: %v", warm, err)
	}

	warmStat, err := os.Stat(output)
	if err != nil || !warmStat.ModTime().Equal(stat.ModTime()) {
		t.Fatal("warm build rewrote output")
	}

	write("child.scene.yaml", "version: 1\nroot: {id: child, components: {logic.bool: {initial: true}}}")

	changed, err := assetbuild.Build(context.Background(), config, options)
	if err != nil || changed.Executed != 2 {
		t.Fatalf("referenced edit report %+v: %v", changed, err)
	}

	after, err := os.ReadFile(output)
	if err != nil || bytes.Equal(before, after) {
		t.Fatal("prefab edit did not update parent code")
	}

	write("child.scene.yaml", "version: 1\nroot: {id: child, unsupported: true}")

	if _, err := assetbuild.Build(context.Background(), config, options); err == nil {
		t.Fatal("bad referenced scene accepted")
	}

	preserved, err := os.ReadFile(output)
	if err != nil || !bytes.Equal(after, preserved) {
		t.Fatal("failed build changed published code")
	}

	if err := os.Remove(filepath.Join(project, "child.scene.yaml")); err != nil {
		t.Fatal(err)
	}

	outside := filepath.Join(t.TempDir(), "child.scene.yaml")
	if err := os.WriteFile(outside, []byte("version: 1\nroot: {id: child}"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.Symlink(outside, filepath.Join(project, "child.scene.yaml")); err != nil {
		t.Fatal(err)
	}

	if _, err := assetbuild.Build(context.Background(), config, options); err == nil {
		t.Fatal("referenced symlink accepted")
	}
}
