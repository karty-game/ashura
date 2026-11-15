//go:build !js

package assetbuild

import (
	"os"
	"path/filepath"
	"testing"
)

//nolint:paralleltest // The fake mise fixture changes process environment.
func TestManagedLibraryTracksReadOnlyFileWithoutExecutingIt(t *testing.T) {
	fixture := newManagedMiseFixture(t)
	fixture.spec.Executable = "lib/component.so"
	fixture.spec.Components = nil

	library := filepath.Join(fixture.root, "lib", "component.so")
	if err := os.Chmod(library, 0o600); err != nil {
		t.Fatal(err)
	}

	ctx := managedMiseTestContext(t, t.Context())
	for range 2 {
		path, err := ctx.ManagedLibrary(fixture.spec)
		if err != nil || path != library {
			t.Fatal(path, err)
		}
	}

	if len(fixture.calls(t)) != 2 {
		t.Fatal("library installation was repeated")
	}

	found := false

	for _, dep := range ctx.deps {
		if dep.Kind == libraryDependency && dep.Name == library {
			found = true
		}
	}

	if !found {
		t.Fatal("library content was not tracked")
	}
}
