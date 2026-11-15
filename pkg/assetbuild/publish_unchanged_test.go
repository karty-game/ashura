//go:build !js

package assetbuild

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUnchangedPublicationSkipsJournalAndRepairsOutputs(t *testing.T) {
	t.Parallel()
	project := t.TempDir()
	fixtureWrite(t, project, "assets/source.txt", []byte("source"))

	var calls [3]int

	config, options := fixtureConfig(&calls), Options{Project: project, Identity: "unchanged-publication"}
	if _, err := Build(t.Context(), config, options); err != nil {
		t.Fatal(err)
	}

	// A directory at the journal path makes any journal write fail. An unchanged
	// publication must not touch it; recovery is tested separately through Build.
	run, err := newRunner(config, options)
	if err != nil {
		t.Fatal(err)
	}

	old, err := run.oldOwnership()
	if err != nil {
		t.Fatal(err)
	}

	files := make(Bundle)
	for _, name := range old.Outputs {
		files[name], err = readProjectFile(project, "generated/assets/"+name)
		if err != nil {
			t.Fatal(err)
		}
	}

	journal := filepath.Join(project, journalName)
	if err := os.Mkdir(journal, 0o700); err != nil {
		t.Fatal(err)
	}

	if err := run.publish(files); err != nil {
		t.Fatalf("unchanged publication attempted journal I/O: %v", err)
	}

	if run.report.Unchanged != len(files) || run.report.Written != 0 {
		t.Fatalf("unchanged publication counters: %+v", run.report)
	}

	if err := os.Remove(journal); err != nil {
		t.Fatal(err)
	}

	for _, damage := range []string{"edit", "delete"} {
		name := old.Outputs[0]
		full := filepath.Join(project, "generated/assets", name)

		if damage == "edit" {
			fixtureWrite(t, project, "generated/assets/"+name, []byte("damaged"))
		} else if err := os.Remove(full); err != nil {
			t.Fatal(err)
		}

		report, err := Build(t.Context(), config, options)
		if err != nil || report.Executed != 0 || report.Written != 1 {
			t.Fatalf("repair %s: %+v %v", damage, report, err)
		}

		got, err := readBounded(full)
		if err != nil || string(got) != string(files[name]) {
			t.Fatalf("repair %s contents: %q %v", damage, got, err)
		}
	}
}

func TestEmptyPublicationEstablishesOwnership(t *testing.T) {
	t.Parallel()

	project := t.TempDir()

	if _, err := Build(t.Context(), Config{Output: "generated/assets"}, Options{Project: project, Identity: "empty"}); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(project, "generated/assets", manifestName)); err != nil {
		t.Fatal(err)
	}
}
