//go:build !js

package assetbuild

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func libraryFixture(path string, calls *int) Config {
	return Config{Output: "generated", Plugins: []Plugin{fixturePlugin(func(plan *Plan) error {
		refs, err := plan.Add(
			Action{
				ID:        "native-bake",
				Processor: "fixture",
				Revision:  "1",
				Outputs:   []Slot{{Name: "files", Schema: "files@1"}},
				Run: func(ctx *Context) (Result, error) {
					*calls++

					if _, err := ctx.TrackLibrary(path); err != nil {
						return nil, err
					}

					data, err := json.Marshal(Bundle{"prepared.bin": []byte("baked")})

					return Result{"files": data}, err
				},
			},
		)
		if err == nil {
			plan.Publish(refs[0])
		}

		return err
	})}}
}
func TestLibraryContentInvalidatesWarmCacheWithoutExecutablePermission(t *testing.T) {
	t.Parallel()
	project, external := t.TempDir(), t.TempDir()

	path := filepath.Join(external, "native-library")
	if err := os.WriteFile(path, []byte("version one"), 0o600); err != nil {
		t.Fatal(err)
	}

	calls := 0
	config := libraryFixture(path, &calls)
	options := Options{Project: project, Identity: "native-library-fixture"}
	build := func(want int) {
		t.Helper()

		report, err := Build(t.Context(), config, options)
		if err != nil || report.Executed != want {
			t.Fatal(report, err, want)
		}
	}
	build(1)
	build(0)

	if err := os.WriteFile(path, []byte("version two"), 0o600); err != nil {
		t.Fatal(err)
	}

	build(1)
	build(0)

	if calls != 2 {
		t.Fatal(calls)
	}

	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}

	if _, err := Build(t.Context(), config, options); err == nil {
		t.Fatal("missing library reused cached bake")
	}
}
func TestLibraryAdmissionPreservesExternalAndProjectPathBoundaries(t *testing.T) {
	t.Parallel()

	for _, path := range []string{"relative.so", t.TempDir()} {
		calls := 0
		if _, err := Build(t.Context(), libraryFixture(path, &calls), Options{Project: t.TempDir(), Identity: "bad-library"}); err == nil {
			t.Fatal("invalid library path admitted", path)
		}
	}
}
func TestLibrarySymlinkRetargetInvalidatesIdenticalContent(t *testing.T) {
	t.Parallel()
	external := t.TempDir()

	first, second, link := filepath.Join(external, "first"), filepath.Join(external, "second"), filepath.Join(external, "library")
	for _, path := range []string{first, second} {
		if err := os.WriteFile(path, []byte("same"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	if err := os.Symlink(first, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	calls := 0

	config, options := libraryFixture(link, &calls), Options{Project: t.TempDir(), Identity: "library-link"}
	if _, err := Build(t.Context(), config, options); err != nil {
		t.Fatal(err)
	}

	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}

	if err := os.Symlink(second, link); err != nil {
		t.Fatal(err)
	}

	report, err := Build(t.Context(), config, options)
	if err != nil || report.Executed != 1 || calls != 2 {
		t.Fatal(report, calls, err)
	}
}
