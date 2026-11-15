//go:build !js

package assetbuild

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const cacheSchema = 1

type cachedOutput struct {
	Hash string `json:"hash"`
	Size int    `json:"size"`
}

type cacheRecord struct {
	Version int                     `json:"version"`
	Base    string                  `json:"base"`
	Key     string                  `json:"key"`
	Deps    []dependency            `json:"deps"`
	Outputs map[string]cachedOutput `json:"outputs"`
}

func (run *runner) actionBase(action Action) (string, error) {
	type input struct {
		Ref  Ref    `json:"ref"`
		Hash string `json:"hash"`
	}

	inputs := make([]input, 0, len(action.Inputs))
	for _, ref := range action.Inputs {
		value, exists := run.outputs[ref]
		if !exists {
			return "", fmt.Errorf("missing input %+v: %w", ref, errPlan)
		}

		inputs = append(inputs, input{Ref: ref, Hash: value.hash})
	}

	return jsonDigest(struct {
		Schema    int        `json:"schema"`
		Identity  string     `json:"identity"`
		ID        string     `json:"id"`
		Processor string     `json:"processor"`
		Revision  string     `json:"revision"`
		Options   any        `json:"options"`
		Inputs    []input    `json:"inputs"`
		Artifacts []Artifact `json:"artifacts,omitempty"`
		Outputs   []Slot     `json:"outputs"`
	}{cacheSchema, run.identity, action.ID, action.Processor, action.Revision, action.Options, inputs, action.Artifacts, action.Outputs})
}

func (run *runner) cached(action Action, base string) (Result, []dependency, string, error) {
	keyData, err := run.readCache("candidates/" + base)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, "no cached action", nil
	}

	if err != nil {
		return nil, nil, "", err
	}

	key := string(keyData)
	if !validHash(key) {
		return nil, nil, "invalid cache index", nil
	}

	encoded, err := run.readCache("actions/" + key)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, "missing cache record", nil
	}

	if err != nil {
		return nil, nil, "", err
	}

	var record cacheRecord

	decodeErr := json.Unmarshal(encoded, &record)
	if decodeErr != nil || record.Version != cacheSchema || record.Base != base || record.Key != key {
		//nolint:nilerr // Corrupt local cache records are misses; recompute from declared source inputs.
		return nil, nil, "invalid cache record", nil
	}

	expected, err := jsonDigest([]any{base, record.Deps})
	if err != nil {
		return nil, nil, "", err
	}

	if key != expected || !validDependencies(record.Deps) || !declaredArtifactsPresent(action, record.Deps) {
		return nil, nil, "invalid cache key", nil
	}

	valid, err := run.validateDependencies(record.Deps)
	if err != nil {
		return nil, nil, "", err
	}

	if !valid {
		return nil, nil, "tracked dependency changed", nil
	}

	result, reason, err := run.cachedOutputs(action, record.Outputs)

	return result, record.Deps, reason, err
}

func (run *runner) cachedOutputs(action Action, outputs map[string]cachedOutput) (Result, string, error) {
	if len(outputs) != len(action.Outputs) {
		return nil, "cache output roster changed", nil
	}

	result := make(Result, len(outputs))
	for _, slot := range action.Outputs {
		output, exists := outputs[slot.Name]
		if !exists || !validHash(output.Hash) || output.Size < 0 || output.Size > maxArtifactBytes {
			return nil, "invalid cache output", nil
		}

		data, err := run.readCache("blobs/" + output.Hash)
		if errors.Is(err, os.ErrNotExist) {
			return nil, "missing cache blob", nil
		}

		if err != nil {
			return nil, "", err
		}

		if len(data) != output.Size || digest(data) != output.Hash {
			return nil, "corrupt cache blob", nil
		}

		result[slot.Name] = data
	}

	return result, "verified inputs and outputs", nil
}

func (run *runner) store(base string, result Result, deps []dependency) error {
	deps = canonicalDependencies(deps)

	key, err := jsonDigest([]any{base, deps})
	if err != nil {
		return err
	}

	record := cacheRecord{Version: cacheSchema, Base: base, Key: key, Deps: deps, Outputs: make(map[string]cachedOutput)}

	for name, data := range result {
		hash := digest(data)
		if err := run.writeCache("blobs/"+hash, data); err != nil {
			return err
		}

		record.Outputs[name] = cachedOutput{Hash: hash, Size: len(data)}
	}

	encoded, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("encode action cache: %w", err)
	}

	if err := run.writeCache("actions/"+key, encoded); err != nil {
		return err
	}

	return run.writeCache("candidates/"+base, []byte(key))
}

func canonicalDependencies(deps []dependency) []dependency {
	result := slices.Clone(deps)
	slices.SortFunc(result, func(left, right dependency) int {
		if order := strings.Compare(left.Kind, right.Kind); order != 0 {
			return order
		}

		if order := strings.Compare(left.Name, right.Name); order != 0 {
			return order
		}

		return strings.Compare(left.Value, right.Value)
	})

	return slices.Compact(result)
}

func validHash(value string) bool {
	const hashLength = 64
	if len(value) != hashLength {
		return false
	}

	return strings.IndexFunc(value, func(runeValue rune) bool {
		return runeValue < '0' || runeValue > '9' && runeValue < 'a' || runeValue > 'f'
	}) == -1
}

func (run *runner) readCache(name string) ([]byte, error) {
	full, err := confined(run.cache, name)
	if err != nil {
		return nil, err
	}

	return readBounded(full)
}

func (run *runner) writeCache(name string, data []byte) error {
	full, err := confined(run.cache, name)
	if err != nil {
		return err
	}

	_, err = writeUnchanged(full, data)

	return err
}

func writeUnchanged(full string, data []byte) (bool, error) {
	//nolint:gosec // Callers confine this compiler-owned path and reject symlink components before publication.
	existing, err := os.ReadFile(full)
	if err == nil && bytes.Equal(existing, data) {
		return false, nil
	}

	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("inspect generated file: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
		return false, fmt.Errorf("create generated parent: %w", err)
	}

	file, err := os.CreateTemp(filepath.Dir(full), ".asset-write-*")
	if err != nil {
		return false, fmt.Errorf("stage generated file: %w", err)
	}
	defer func() { _ = os.Remove(file.Name()) }()

	if _, err := file.Write(data); err != nil {
		_ = file.Close()

		return false, fmt.Errorf("write generated file: %w", err)
	}

	if err := file.Close(); err != nil {
		return false, fmt.Errorf("close generated file: %w", err)
	}

	if err := os.Rename(file.Name(), full); err != nil {
		return false, fmt.Errorf("publish generated file: %w", err)
	}

	return true, nil
}
