//go:build !js

package assetbuild

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

const manifestName = ".engine-assets.json"

type ownership struct {
	//nolint:tagliatelle // Retain the legacy ownership manifest's serialized field names for compatibility.
	Version int `json:"Version"`
	//nolint:tagliatelle // Retain the legacy ownership manifest's serialized field names for compatibility.
	Outputs []string `json:"Outputs"`
}

type publication struct {
	Version int      `json:"version"`
	Output  string   `json:"output"`
	Old     []string `json:"old"`
	Files   Bundle   `json:"files"`
}

func generatedName(name string) bool {
	return fs.ValidPath(name) && name != "." && name != manifestName && !strings.HasPrefix(name, manifestName+"/") &&
		!strings.ContainsAny(name, "\\:\x00") && !strings.HasPrefix(name, ".asset-")
}

func (run *runner) oldOwnership() (ownership, error) {
	var old ownership

	full, err := confined(run.output, manifestName)
	if err != nil {
		return old, err
	}

	data, err := readBounded(full)
	if errors.Is(err, os.ErrNotExist) {
		entries, readErr := os.ReadDir(run.output)
		if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
			return old, fmt.Errorf("inspect generated output: %w", readErr)
		}

		for _, entry := range entries {
			if !containsPath(filepath.Join(run.output, entry.Name()), run.buildState) {
				return old, fmt.Errorf("output directory is nonempty and not owned by assetbuild: %w", errPath)
			}
		}

		return ownership{Version: cacheSchema}, nil
	}

	if err != nil {
		return old, err
	}

	if err := json.Unmarshal(data, &old); err != nil {
		return old, fmt.Errorf("read output ownership: %w", err)
	}

	if old.Version != cacheSchema {
		return old, fmt.Errorf("unsupported ownership version %d: %w", old.Version, errPlan)
	}

	for _, name := range old.Outputs {
		if !generatedName(name) {
			return old, fmt.Errorf("invalid owned output %q: %w", name, errPath)
		}

		if _, err := run.publicationPath(name); err != nil {
			return old, err
		}
	}

	return old, nil
}

func (run *runner) publish(files Bundle) error {
	old, err := run.oldOwnership()
	if err != nil {
		return err
	}

	if err := run.validatePublication(files, old.Outputs); err != nil {
		return err
	}

	unchanged, err := run.publicationUnchanged(files, old.Outputs)
	if err != nil {
		return err
	}

	if unchanged {
		for _, data := range files {
			run.report.Unchanged++
			run.report.Bytes += len(data)
		}

		return nil
	}

	relative, err := filepath.Rel(run.project, run.output)
	if err != nil {
		return fmt.Errorf("publication output identity: %w", err)
	}

	journal, err := run.stagePublication(filepath.ToSlash(relative), old.Outputs, files)
	if err != nil {
		return err
	}

	encoded, err := json.Marshal(journal)
	if err != nil {
		return fmt.Errorf("encode publication journal: %w", err)
	}

	if len(encoded) > maxArtifactBytes {
		return fmt.Errorf("publication journal exceeds %d-byte recovery limit: %w", maxArtifactBytes, errPath)
	}

	journalPath, err := confined(run.project, journalName)
	if err != nil {
		return err
	}

	if _, err := writeUnchanged(journalPath, encoded); err != nil {
		return err
	}

	if err := run.finishStagedPublication(journal); err != nil {
		return err
	}

	if err := os.Remove(journalPath); err != nil {
		return fmt.Errorf("complete publication journal: %w", err)
	}

	return run.cleanupPublication(journal)
}

// Verify contents, not timestamps: edited or deleted outputs must be repaired.
// The caller holds the project lock and validates ownership and paths first.
func (run *runner) publicationUnchanged(files Bundle, previous []string) (bool, error) {
	if len(files) != len(previous) {
		return false, nil
	}

	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}

	slices.Sort(names)

	old := slices.Clone(previous)
	slices.Sort(old)

	if !slices.Equal(names, old) {
		return false, nil
	}

	// A first empty build still needs to establish output ownership.
	manifest, err := confined(run.output, manifestName)
	if err != nil {
		return false, err
	}

	if _, err := readBounded(manifest); errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, err
	}

	for _, name := range previous {
		data := files[name]

		full, err := run.publicationPath(name)
		if err != nil {
			return false, err
		}

		existing, err := readBounded(full)
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, errPath) {
			return false, nil
		}

		if err != nil {
			return false, err
		}

		if !bytes.Equal(existing, data) {
			return false, nil
		}
	}

	return true, nil
}

func (run *runner) validatePublication(files Bundle, previous []string) error {
	if err := validateOutputRoster(files); err != nil {
		return err
	}

	for _, name := range previous {
		if !generatedName(name) {
			return fmt.Errorf("invalid previous output %q: %w", name, errPath)
		}

		full, err := run.publicationPath(name)
		if err != nil {
			return err
		}

		info, err := os.Lstat(full)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect previous output %s: %w", name, err)
		}

		if err == nil && !info.Mode().IsRegular() {
			return fmt.Errorf("previous output %q is not a regular file: %w", name, errPath)
		}
	}

	for name, data := range files {
		if !generatedName(name) || len(data) > maxArtifactBytes {
			return fmt.Errorf("invalid generated output %q: %w", name, errPath)
		}

		full, err := run.publicationPath(name)
		if err != nil {
			return err
		}

		info, err := os.Lstat(full)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect output %s: %w", name, err)
		}

		if err == nil && (!info.Mode().IsRegular() || !slices.Contains(previous, name)) {
			return fmt.Errorf("refuse to replace unowned/non-file output %q: %w", name, errPath)
		}
	}

	return nil
}

func validateOutputRoster(files Bundle) error {
	for name := range files {
		if !generatedName(name) {
			return fmt.Errorf("invalid generated name %q: %w", name, errPath)
		}

		for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
			if _, exists := files[parent]; exists {
				return fmt.Errorf("generated file/directory conflict %q and %q: %w", parent, name, errPath)
			}
		}
	}

	return nil
}

func (run *runner) finishPublication(journal publication) error {
	names := make([]string, 0, len(journal.Files))
	for name := range journal.Files {
		names = append(names, name)
	}

	return run.finishPublicationFiles(journal.Old, names, func(name string) ([]byte, error) { return journal.Files[name], nil })
}

func (run *runner) finishPublicationFiles(previous, names []string, read func(string) ([]byte, error)) error {
	slices.Sort(names)

	manifest, err := json.MarshalIndent(ownership{Version: cacheSchema, Outputs: names}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode output ownership: %w", err)
	}

	if len(manifest) > maxArtifactBytes {
		return fmt.Errorf("output ownership exceeds byte limit: %w", errPath)
	}

	for _, name := range names {
		data, err := read(name)
		if err != nil {
			return err
		}

		full, err := run.publicationPath(name)
		if err != nil {
			return err
		}

		changed, err := writeUnchanged(full, data)
		if err != nil {
			return err
		}

		if changed {
			run.report.Written++
		} else {
			run.report.Unchanged++
		}

		run.report.Bytes += len(data)
	}

	for _, name := range previous {
		if !slices.Contains(names, name) {
			full, err := run.publicationPath(name)
			if err != nil {
				return err
			}

			if err := os.Remove(full); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("prune stale generated output: %w", err)
			}

			run.report.Removed++
		}
	}

	full, err := confined(run.output, manifestName)
	if err != nil {
		return err
	}

	_, err = writeUnchanged(full, manifest)

	return err
}

func (run *runner) recoverPublication() error {
	full, err := confined(run.project, journalName)
	if err != nil {
		return err
	}

	encoded, err := readBounded(full)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}

	if err != nil {
		return err
	}

	var identity struct {
		Version int    `json:"version"`
		Output  string `json:"output"`
	}
	if err := json.Unmarshal(encoded, &identity); err != nil || identity.Version != cacheSchema && identity.Version != publicationSchema {
		return fmt.Errorf("invalid publication journal; preserve it for recovery: %w", errPlan)
	}

	output, err := confined(run.project, identity.Output)
	if err != nil || output == run.project || containsPath(run.buildState, output) {
		return fmt.Errorf("invalid publication journal output: %w", errPath)
	}

	current := run.output

	run.output = output
	defer func() { run.output = current }()

	var staged stagedPublication
	if identity.Version == publicationSchema {
		if err := json.Unmarshal(encoded, &staged); err != nil {
			return fmt.Errorf("invalid staged publication journal: %w", errPlan)
		}

		if err := run.finishStagedPublication(staged); err != nil {
			return err
		}
	} else if err := run.finishEmbeddedPublication(encoded); err != nil {
		return err
	}

	if err := os.Remove(full); err != nil {
		return fmt.Errorf("finish interrupted publication: %w", err)
	}

	if identity.Version == publicationSchema {
		return run.cleanupPublication(staged)
	}

	return nil
}

func (run *runner) finishEmbeddedPublication(encoded []byte) error {
	var journal publication
	if err := json.Unmarshal(encoded, &journal); err != nil {
		return fmt.Errorf("invalid embedded publication journal: %w", errPlan)
	}

	owned := slices.Clone(journal.Old)
	for name := range journal.Files {
		owned = append(owned, name)
	}

	if err := run.validatePublication(journal.Files, owned); err != nil {
		return err
	}

	return run.finishPublication(journal)
}
