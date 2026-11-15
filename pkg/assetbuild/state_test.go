//go:build !js

package assetbuild

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestGeneratedRootSharesDisposableCache(t *testing.T) {
	t.Parallel()
	project := t.TempDir()
	fixtureWrite(t, project, "assets/source.txt", []byte("hello"))

	var calls [3]int

	config, options := fixtureConfig(&calls), Options{Project: project, Identity: "generated-root"}
	config.Output = "generated"

	for _, wantExecuted := range []int{3, 0} {
		report, err := Build(t.Context(), config, options)
		if err != nil || report.Executed != wantExecuted {
			t.Fatalf("build: %+v %v, want %d executed", report, err, wantExecuted)
		}
	}

	for _, name := range []string{"generated/value.bin", lockName, cacheDirectory + "/actions"} {
		if _, err := os.Stat(filepath.Join(project, name)); err != nil {
			t.Fatalf("missing output/cache %s: %v", name, err)
		}
	}

	if _, err := os.Lstat(filepath.Join(project, ".karty")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("created CLI state: %v", err)
	}

	if err := os.RemoveAll(filepath.Join(project, "generated")); err != nil {
		t.Fatal(err)
	}

	report, err := Build(t.Context(), config, options)
	if err != nil || report.Executed != 3 {
		t.Fatalf("rebuild after deleting outputs and cache: %+v %v", report, err)
	}
}

func TestExistingCLIStateIsUntouched(t *testing.T) {
	t.Parallel()
	project := t.TempDir()
	fixtureWrite(t, project, "assets/source.txt", []byte("hello"))

	cliJournal := ".karty/asset-build/publication.json"
	fixtureWrite(t, project, cliJournal, []byte("unrelated CLI state"))

	var calls [3]int

	if _, err := Build(t.Context(), fixtureConfig(&calls), Options{Project: project, Identity: "separate-state"}); err != nil {
		t.Fatal(err)
	}

	data, err := fixtureRead(project, cliJournal)
	if err != nil || string(data) != "unrelated CLI state" {
		t.Fatalf("modified CLI state: %q %v", data, err)
	}
}

func TestGeneratedPublicationCannotOwnBuildState(t *testing.T) {
	t.Parallel()
	project := t.TempDir()

	run, err := newRunner(Config{Output: "generated"}, Options{Project: project, Identity: "reserved-state"})
	if err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"cache", "cache/assets/v1/actions/entry", "cache/asset-build/lock", "cache/asset-build/publication.json"} {
		if err := run.publish(Bundle{name: []byte("overwrite")}); !errors.Is(err, errPath) {
			t.Fatalf("accepted reserved output %s: %v", name, err)
		}
	}

	marker, err := json.Marshal(ownership{Version: cacheSchema, Outputs: []string{"cache/keep"}})
	if err != nil {
		t.Fatal(err)
	}

	fixtureWrite(t, project, "generated/"+manifestName, marker)
	fixtureWrite(t, project, "generated/cache/keep", []byte("keep"))

	if _, err := run.oldOwnership(); !errors.Is(err, errPath) {
		t.Fatalf("accepted stale ownership of cache: %v", err)
	}

	data, err := fixtureRead(project, "generated/cache/keep")
	if err != nil || string(data) != "keep" {
		t.Fatalf("changed reserved state: %q %v", data, err)
	}
}

func TestGeneratedRootPublicationRecoveryPreservesCache(t *testing.T) {
	t.Parallel()
	project := t.TempDir()
	journal := publication{Version: cacheSchema, Output: "generated", Files: Bundle{"model.bin": []byte("complete")}}

	encoded, err := json.Marshal(journal)
	if err != nil {
		t.Fatal(err)
	}

	fixtureWrite(t, project, journalName, encoded)
	fixtureWrite(t, project, cacheDirectory+"/keep", []byte("cache"))

	run, err := newRunner(Config{Output: "generated/assets"}, Options{Project: project, Identity: "recover-parent"})
	if err != nil {
		t.Fatal(err)
	}

	if err := run.recoverPublication(); err != nil {
		t.Fatal(err)
	}

	for name, want := range map[string]string{"generated/model.bin": "complete", cacheDirectory + "/keep": "cache"} {
		data, err := fixtureRead(project, name)
		if err != nil || string(data) != want {
			t.Fatalf("recovered output/state %s: %q %v", name, data, err)
		}
	}
}
