//go:build !js

package assetbuild

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestCorruptDependencyRecordsAreCacheMisses(t *testing.T) {
	t.Parallel()

	for _, item := range []dependency{
		{Kind: "unknown", Name: "assets/source.txt", Value: digest([]byte("hello"))},
		{Kind: "file", Name: "../escape", Value: digest([]byte("hello"))},
		{Kind: "glob", Name: "assets/[", Value: digest([]byte("hello"))},
		{Kind: toolDependency, Name: "", Value: digest([]byte("tool"))},
		{Kind: toolDependency, Name: "bad\x00tool", Value: digest([]byte("tool"))},
		{Kind: toolDependency, Name: "./processor.exe", Value: "invalid executable fingerprint"},
	} {
		t.Run(item.Kind+item.Name, func(t *testing.T) {
			t.Parallel()
			project := t.TempDir()
			fixtureWrite(t, project, "assets/source.txt", []byte("hello"))

			var calls [3]int

			config, options := fixtureConfig(&calls), Options{Project: project, Identity: "corrupt-dependency"}
			if _, err := Build(t.Context(), config, options); err != nil {
				t.Fatal(err)
			}

			corruptDependency(t, project, item)

			report, err := Build(t.Context(), config, options)
			if err != nil || report.Executed != 1 || report.Reused != 2 {
				t.Fatalf("invalid cache dependency: report=%+v err=%v", report, err)
			}
		})
	}
}

func corruptDependency(t *testing.T, project string, item dependency) {
	t.Helper()

	paths, err := filepath.Glob(filepath.Join(project, "generated/cache/assets/v1/actions/*"))
	if err != nil {
		t.Fatal(err)
	}

	for _, full := range paths {
		data, err := readBounded(full)
		if err != nil {
			t.Fatal(err)
		}

		var record cacheRecord
		if err := json.Unmarshal(data, &record); err != nil {
			t.Fatal(err)
		}

		if len(record.Deps) == 0 {
			continue
		}

		record.Deps = []dependency{item}

		record.Key, err = jsonDigest([]any{record.Base, record.Deps})
		if err != nil {
			t.Fatal(err)
		}

		data, err = json.Marshal(record)
		if err != nil {
			t.Fatal(err)
		}

		fixtureWrite(t, project, "generated/cache/assets/v1/actions/"+record.Key, data)
		fixtureWrite(t, project, "generated/cache/assets/v1/candidates/"+record.Base, []byte(record.Key))

		return
	}

	t.Fatal("no tracked source action found")
}

func TestNoCacheDoesNotReadCorruptActionEntries(t *testing.T) {
	t.Parallel()
	project := t.TempDir()
	fixtureWrite(t, project, "assets/source.txt", []byte("hello"))

	var calls [3]int

	config, options := fixtureConfig(&calls), Options{Project: project, Identity: "no-cache"}
	if _, err := Build(t.Context(), config, options); err != nil {
		t.Fatal(err)
	}

	cache := filepath.Join(project, "generated/cache/assets/v1")
	if err := os.RemoveAll(cache); err != nil {
		t.Fatal(err)
	}

	fixtureWrite(t, project, "generated/cache/assets/v1/actions", []byte("not a directory"))

	options.NoCache = true

	report, err := Build(t.Context(), config, options)
	if err != nil || report.Executed != 3 {
		t.Fatalf("cache bypass read entries: %+v %v", report, err)
	}
}
