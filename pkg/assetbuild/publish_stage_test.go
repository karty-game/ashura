//go:build !js

package assetbuild

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func stagedFixture(t *testing.T) (*runner, stagedPublication) {
	t.Helper()

	run, err := newRunner(Config{Output: "generated/assets"}, Options{Project: t.TempDir(), Identity: "staged-publication"})
	if err != nil {
		t.Fatal(err)
	}

	journal, err := run.stagePublication(
		"generated/assets",
		[]string{"old.bin"},
		Bundle{"a.bin": []byte("first complete"), "z.bin": []byte("last complete")},
	)
	if err != nil {
		t.Fatal(err)
	}

	fixtureWrite(t, run.project, "generated/assets/old.bin", []byte("old"))
	fixtureWrite(t, run.project, "generated/assets/a.bin", []byte("partial"))
	fixtureWrite(t, run.project, "generated/assets/unrelated.bin", []byte("keep"))

	return run, journal
}

func writeStagedJournal(t *testing.T, run *runner, journal stagedPublication) {
	t.Helper()

	encoded, err := json.Marshal(journal)
	if err != nil {
		t.Fatal(err)
	}

	fixtureWrite(t, run.project, journalName, encoded)
}

func TestStagedPublicationRecoversAndCleansOnlyCommittedHashes(t *testing.T) {
	t.Parallel()
	run, journal := stagedFixture(t)
	writeStagedJournal(t, run, journal)

	orphan := digest([]byte("orphan"))
	fixtureWrite(t, run.project, publicationBlobs+"/"+orphan, []byte("orphan"))
	fixtureWrite(t, run.project, cacheDirectory+"/blobs/"+journal.Files["a.bin"].Hash, []byte("shared cache"))

	config := Config{Output: "generated/assets", Plugins: []Plugin{fixturePlugin(func(*Plan) error { return errStopAfterRecovery })}}
	if _, err := Build(t.Context(), config, Options{Project: run.project, Identity: "recovery"}); !errors.Is(err, errStopAfterRecovery) {
		t.Fatalf("recovery did not precede processing: %v", err)
	}

	for name, want := range map[string]string{"a.bin": "first complete", "z.bin": "last complete", "unrelated.bin": "keep"} {
		data, err := fixtureRead(run.project, "generated/assets/"+name)
		if err != nil || string(data) != want {
			t.Fatalf("recovered %s: %q %v", name, data, err)
		}
	}

	for _, name := range []string{journalName, "generated/assets/old.bin"} {
		if _, err := os.Lstat(filepath.Join(run.project, name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("completed state remains %s: %v", name, err)
		}
	}

	for _, reference := range journal.Files {
		if _, err := os.Lstat(filepath.Join(run.project, publicationBlobs, reference.Hash)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("staged blob remains: %v", err)
		}
	}

	for name, want := range map[string]string{publicationBlobs + "/" + orphan: "orphan", cacheDirectory + "/blobs/" + journal.Files["a.bin"].Hash: "shared cache"} {
		data, err := fixtureRead(run.project, name)
		if err != nil || string(data) != want {
			t.Fatalf("cleanup changed unrelated state %s: %q %v", name, data, err)
		}
	}
}

func TestStagedPublicationPreflightsAllPayloadsAndPaths(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"missing", "tampered", "wrong-size", "negative-size", "oversized", "invalid-hash", "escaping-hash", "symlink-file", "symlink-directory", "escaping-name", "reserved-name", "roster-conflict", "escaping-old", "stale-directory", "build-state-name"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			run, journal := stagedFixture(t)
			damageStagedPublication(t, name, run, &journal)

			writeStagedJournal(t, run, journal)

			if err := run.recoverPublication(); err == nil {
				t.Fatal("accepted malformed staged publication")
			}

			for output, want := range map[string]string{"a.bin": "partial", "old.bin": "old", "unrelated.bin": "keep"} {
				if name == "stale-directory" && output == "old.bin" {
					continue
				}

				data, err := fixtureRead(run.project, "generated/assets/"+output)
				if err != nil || string(data) != want {
					t.Fatalf("changed output before complete preflight %s: %q %v", output, data, err)
				}
			}

			if _, err := os.Lstat(filepath.Join(run.output, "z.bin")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("published final output before validation: %v", err)
			}

			if _, err := os.Lstat(filepath.Join(run.project, journalName)); err != nil {
				t.Fatalf("discarded recoverable journal: %v", err)
			}
		})
	}
}

func damageStagedPublication(t *testing.T, name string, run *runner, journal *stagedPublication) {
	t.Helper()

	reference := journal.Files["z.bin"]
	blob := filepath.Join(run.project, publicationBlobs, reference.Hash)

	switch name {
	case "missing":
		if err := os.Remove(blob); err != nil {
			t.Fatal(err)
		}
	case "tampered":
		fixtureWrite(t, run.project, publicationBlobs+"/"+reference.Hash, []byte("last tampered"))
	case "wrong-size":
		reference.Size++
		journal.Files["z.bin"] = reference
	case "negative-size":
		reference.Size = -1
		journal.Files["z.bin"] = reference
	case "oversized":
		reference.Size = maxArtifactBytes + 1
		journal.Files["z.bin"] = reference
	case "invalid-hash":
		reference.Hash = "g" + reference.Hash[1:]
		journal.Files["z.bin"] = reference
	case "escaping-hash":
		reference.Hash = "../" + reference.Hash
		journal.Files["z.bin"] = reference
	case "symlink-file":
		if err := os.Remove(blob); err != nil {
			t.Fatal(err)
		}

		outside := filepath.Join(t.TempDir(), "payload")
		if err := os.WriteFile(outside, []byte("last complete"), 0o600); err != nil {
			t.Fatal(err)
		}

		if err := os.Symlink(outside, blob); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
	case "symlink-directory":
		directory := filepath.Dir(blob)

		outside := filepath.Join(t.TempDir(), "payloads")
		if err := os.Rename(directory, outside); err != nil {
			t.Fatal(err)
		}

		if err := os.Symlink(outside, directory); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
	case "escaping-name":
		journal.Files["../escape.bin"] = reference
	case "reserved-name":
		journal.Files["../cache/stolen.bin"] = reference
	case "roster-conflict":
		journal.Files["a.bin/child"] = reference
	case "escaping-old":
		journal.Old = append(journal.Old, "../stale.bin")
	case "stale-directory":
		if err := os.Remove(filepath.Join(run.output, "old.bin")); err != nil {
			t.Fatal(err)
		}

		if err := os.Mkdir(filepath.Join(run.output, "old.bin"), 0o700); err != nil {
			t.Fatal(err)
		}
	case "build-state-name":
		journal.Output = "generated"
		journal.Files["cache/forbidden.bin"] = reference
	}
}

//nolint:paralleltest // Keep the 200 MiB family regression serialized to bound test memory.
func TestPublicationLargeFamilyColdIncrementalAndUnchanged(t *testing.T) {
	// Serialize this memory-intensive regression: each file is bounded, while its
	// old base64 journal would exceed the unchanged 256 MiB journal admission bound.
	run, err := newRunner(Config{Output: "generated/assets"}, Options{Project: t.TempDir(), Identity: "large-publication"})
	if err != nil {
		t.Fatal(err)
	}

	files := make(Bundle)
	encodedPayloadBytes := 0

	for index := range 4 {
		data := make([]byte, maxArtifactBytes/5)
		data[0] = byte(index)
		data[len(data)-1] = byte(index + 1)
		files[fmt.Sprintf("payload-%d.bin", index)] = data
		encodedPayloadBytes += (len(data) + 2) / 3 * 4
	}

	if encodedPayloadBytes <= maxArtifactBytes {
		t.Fatal("fixture does not exceed legacy journal bound")
	}

	if err := run.publish(files); err != nil {
		t.Fatalf("cold large publication: %v", err)
	}

	if run.report.Written != len(files) {
		t.Fatalf("cold publication counters: %+v", run.report)
	}

	files["payload-0.bin"][0] = 42
	files["new.bin"] = []byte("incremental")

	run.report = Report{}
	if err := run.publish(files); err != nil {
		t.Fatalf("incremental large publication: %v", err)
	}

	if run.report.Written != 2 || run.report.Unchanged != 3 {
		t.Fatalf("incremental counters: %+v", run.report)
	}

	for name, want := range files {
		data, err := readBounded(filepath.Join(run.output, name))
		if err != nil || !bytes.Equal(data, want) {
			t.Fatalf("large family output %s differs: %v", name, err)
		}
	}

	entries, err := os.ReadDir(filepath.Join(run.project, publicationBlobs))
	if err != nil || len(entries) != 0 {
		t.Fatalf("completed publication staging remains: %d %v", len(entries), err)
	}
	// Existing no-op behavior must remain independent of journal writes.
	if err := os.Mkdir(filepath.Join(run.project, journalName), 0o700); err != nil {
		t.Fatal(err)
	}

	run.report = Report{}
	if err := run.publish(files); err != nil {
		t.Fatalf("unchanged large publication touched journal: %v", err)
	}

	if run.report.Written != 0 || run.report.Unchanged != len(files) {
		t.Fatalf("unchanged counters: %+v", run.report)
	}
}
