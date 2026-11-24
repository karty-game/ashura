//go:build !js

package collisionbuild

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/karty-game/ashura/internal/modelbuild/testfixture"
	"github.com/karty-game/ashura/pkg/assetbuild"
	modelbuild "github.com/karty-game/ashura/pkg/model/build"
)

func TestStaticModelsPublishesAndCachesPreparedGeometry(t *testing.T) {
	t.Parallel()

	project, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(project, "proxy.glb"), testfixture.Scene(), 0o600); err != nil {
		t.Fatal(err)
	}

	config := assetbuild.Config{
		Output: "generated",
		Pipelines: []assetbuild.Pipeline{{
			Name:   "collision",
			Input:  []string{"proxy.glb"},
			Import: []assetbuild.Plugin{modelbuild.PreparedGLB()},
			Output: []assetbuild.Plugin{StaticModels()},
		}},
	}
	options := assetbuild.Options{Project: project, Identity: "collision-test-v1"}

	first, err := assetbuild.Build(t.Context(), config, options)
	if err != nil || first.Written != 1 || first.Executed != 2 {
		t.Fatalf("initial preparation = %+v, %v", first, err)
	}

	compiled, err := os.ReadFile(filepath.Join(project, "generated", "colliders", "mesh_Proxy.go"))
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Contains(compiled, []byte("func Proxy() *collision.Mesh")) ||
		!bytes.Contains(compiled, []byte("collision.PreparedMeshV1")) {
		t.Fatalf("unexpected collision catalog: %s", compiled)
	}

	cached, err := assetbuild.Build(t.Context(), config, options)
	if err != nil || cached.Reused != 2 || cached.Unchanged != 1 {
		t.Fatalf("cached preparation = %+v, %v", cached, err)
	}
}
