//go:build !js

package assetbuild

import (
	"errors"
	"fmt"
	"os"
	"slices"
)

const (
	publicationSchema = 2
	publicationBlobs  = stateDirectory + "/asset-build/publication-blobs"
)

// A v2 journal holds only bounded file identities, never base64 asset payloads.
// Payloads belong to this project's publication state, not the action cache.
// An interrupted pre-journal stage can leave unreferenced hashes; recovery never
// follows or deletes files that are absent from the committed journal roster.
type stagedPublication struct {
	Version int                     `json:"version"`
	Output  string                  `json:"output"`
	Old     []string                `json:"old"`
	Files   map[string]cachedOutput `json:"files"`
}

func (run *runner) publicationBlob(hash string) (string, error) {
	if !validHash(hash) {
		return "", fmt.Errorf("invalid publication payload hash: %w", errPath)
	}

	return confined(run.project, publicationBlobs+"/"+hash)
}

func (run *runner) stagePublication(output string, previous []string, files Bundle) (stagedPublication, error) {
	journal := stagedPublication{
		Version: publicationSchema,
		Output:  output,
		Old:     previous,
		Files:   make(map[string]cachedOutput, len(files)),
	}
	for name, data := range files {
		if len(data) > maxArtifactBytes {
			return journal, fmt.Errorf("publication payload exceeds byte limit: %w", errPath)
		}

		reference := cachedOutput{Hash: digest(data), Size: len(data)}

		full, err := run.publicationBlob(reference.Hash)
		if err != nil {
			return journal, err
		}

		existing, err := readBounded(full)
		switch {
		case errors.Is(err, os.ErrNotExist):
			if _, err := writeUnchanged(full, data); err != nil {
				return journal, err
			}
		case err != nil:
			return journal, err
		case len(existing) != reference.Size || digest(existing) != reference.Hash:
			return journal, fmt.Errorf("corrupt staged publication payload: %w", errPath)
		}

		journal.Files[name] = reference
	}

	return journal, nil
}

func (run *runner) readPublicationBlob(reference cachedOutput) ([]byte, error) {
	if reference.Size < 0 || reference.Size > maxArtifactBytes {
		return nil, fmt.Errorf("invalid publication payload size: %w", errPath)
	}

	full, err := run.publicationBlob(reference.Hash)
	if err != nil {
		return nil, err
	}

	data, err := readBounded(full)
	if err != nil {
		return nil, err
	}

	if len(data) != reference.Size || digest(data) != reference.Hash {
		return nil, fmt.Errorf("publication payload checksum/size mismatch: %w", errPath)
	}

	return data, nil
}

func (run *runner) finishStagedPublication(journal stagedPublication) error {
	roster := make(Bundle, len(journal.Files))
	owned := slices.Clone(journal.Old)

	names := make([]string, 0, len(journal.Files))
	for name := range journal.Files {
		roster[name] = nil
		owned = append(owned, name)
		names = append(names, name)
	}

	if err := run.validatePublication(roster, owned); err != nil {
		return err
	}
	// Complete hash and realpath preflight precedes every output mutation. One
	// bounded payload is read at a time, so recovery does not retain another family.
	slices.Sort(names)

	for _, name := range names {
		if _, err := run.readPublicationBlob(journal.Files[name]); err != nil {
			return err
		}
	}

	return run.finishPublicationFiles(
		journal.Old,
		names,
		func(name string) ([]byte, error) { return run.readPublicationBlob(journal.Files[name]) },
	)
}

func (run *runner) cleanupPublication(journal stagedPublication) error {
	// Only call after successfully removing the journal. No cache or directory
	// scan is involved, and repeated references to a shared hash are harmless.
	for _, reference := range journal.Files {
		full, err := run.publicationBlob(reference.Hash)
		if err != nil {
			return err
		}

		if err := os.Remove(full); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove completed publication payload: %w", err)
		}
	}

	return nil
}
