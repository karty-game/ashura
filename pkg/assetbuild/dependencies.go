//go:build !js

package assetbuild

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const (
	fileDependency    = "file"
	globDependency    = "glob"
	envDependency     = "env"
	missingDependency = "missing"
)

type dependency struct {
	Kind  string `json:"kind"`
	Name  string `json:"name"`
	Value string `json:"value"`
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)

	return hex.EncodeToString(sum[:])
}

func jsonDigest(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("serialize action identity: %w", err)
	}

	return digest(data), nil
}

// Input returns a copy of a declared upstream output. Undeclared reads fail.
func (ctx *Context) Input(ref Ref) ([]byte, error) {
	ref = (&Plan{namespace: ctx.action.namespace}).scopedRef(ref)
	if !slices.Contains(ctx.action.Inputs, ref) {
		return nil, fmt.Errorf("action %q read undeclared input %+v: %w", ctx.action.ID, ref, errPlan)
	}

	value, exists := ctx.runner.outputs[ref]
	if !exists {
		return nil, fmt.Errorf("action %q input %+v is unavailable: %w", ctx.action.ID, ref, errPlan)
	}

	return slices.Clone(value.data), nil
}

// ReadFile reads a project-confined immutable snapshot and records its content.
// A missing-file lookup is tracked even when the plugin handles os.ErrNotExist.
func (ctx *Context) ReadFile(name string) ([]byte, error) {
	if ctx.expanding {
		return nil, ctx.discoveryError()
	}

	full, err := confined(ctx.runner.project, name)
	if err != nil {
		return nil, err
	}

	if overlaps(full, ctx.runner.output) || overlaps(full, ctx.runner.buildState) {
		return nil, fmt.Errorf("source %q overlaps output/build state: %w", name, errPath)
	}

	relative, err := filepath.Rel(ctx.runner.project, full)
	if err != nil {
		return nil, fmt.Errorf("source identity: %w", err)
	}

	name = filepath.ToSlash(relative)

	data, value, err := ctx.runner.fileSnapshot(name)
	if err == nil || errors.Is(err, os.ErrNotExist) {
		ctx.deps = append(ctx.deps, dependency{Kind: fileDependency, Name: name, Value: value})
	}

	return slices.Clone(data), err
}

// Glob returns sorted matching project paths and tracks additions/removals.
func (ctx *Context) Glob(pattern string) ([]string, error) {
	if ctx.expanding {
		return nil, ctx.discoveryError()
	}

	if err := validateSourceRoots(ctx.runner.project, ctx.runner.output, ctx.runner.buildState, []string{pattern}); err != nil {
		return nil, err
	}

	names, err := globFiles(ctx.runner.project, pattern)
	if err != nil {
		return nil, err
	}

	value, err := jsonDigest(names)
	if err != nil {
		return nil, err
	}

	ctx.deps = append(ctx.deps, dependency{Kind: globDependency, Name: pattern, Value: value})

	return names, nil
}

// Getenv returns a tracked environment value, including whether it is defined.
func (ctx *Context) Getenv(name string) string {
	if ctx.expanding {
		_ = ctx.discoveryError()

		return ""
	}

	value, exists := os.LookupEnv(name)
	identity, _ := jsonDigest([]any{exists, value})
	ctx.deps = append(ctx.deps, dependency{Kind: envDependency, Name: name, Value: identity})

	return value
}

func (run *runner) fileSnapshot(name string) ([]byte, string, error) {
	if run.snapshotMu != nil {
		run.snapshotMu.Lock()
		defer run.snapshotMu.Unlock()
	}

	if snapshot, exists := run.files[name]; exists {
		return snapshot.data, snapshot.hash, snapshot.err
	}

	data, err := readProjectFile(run.project, name)

	value := digest(data)
	if errors.Is(err, os.ErrNotExist) {
		value = missingDependency
	}

	run.files[name] = fileValue{data: data, hash: value, err: err}

	return data, value, err
}

func (run *runner) validateDependencies(deps []dependency) (bool, error) {
	for _, item := range deps {
		value, err := run.dependencyValue(item)
		if err != nil {
			return false, err
		}

		if value != item.Value {
			return false, nil
		}
	}

	return true, nil
}

func validDependencies(deps []dependency) bool {
	artifactCount := 0

	for _, item := range deps {
		if item.Kind == artifactDependency {
			artifactCount++
			if artifactCount > maxPlanSteps {
				return false
			}
		}

		if !validDependency(item) {
			return false
		}
	}

	return true
}

func validDependency(item dependency) bool {
	switch item.Kind {
	case fileDependency:
		return fs.ValidPath(item.Name) && !strings.ContainsAny(item.Name, "\\:") &&
			(item.Value == missingDependency || validHash(item.Value))
	case globDependency:
		_, err := globPrefix(item.Name)

		return err == nil && validHash(item.Value)
	case envDependency:
		return item.Name != "" && !strings.ContainsAny(item.Name, "=\x00") && validHash(item.Value)
	case libraryDependency:
		return filepath.IsAbs(item.Name) && validToolName(item.Name) && (item.Value == toolUnavailable || validHash(item.Value))
	case toolDependency:
		return validToolName(item.Name) && (item.Value == toolUnavailable || validHash(item.Value))
	case artifactDependency:
		_, ok := dependencyArtifact(item)

		return ok
	default:
		return false
	}
}

func (run *runner) dependencyValue(item dependency) (string, error) {
	switch item.Kind {
	case fileDependency:
		_, value, err := run.fileSnapshot(item.Name)
		if errors.Is(err, os.ErrNotExist) {
			return value, nil
		}

		return value, err
	case globDependency:
		names, err := globFiles(run.project, item.Name)
		if err != nil {
			return "", err
		}

		return jsonDigest(names)
	case envDependency:
		value, exists := os.LookupEnv(item.Name)

		return jsonDigest([]any{exists, value})
	case libraryDependency:
		_, value, _ := run.librarySnapshot(item.Name)

		return value, nil
	case toolDependency:
		_, value, _ := run.toolSnapshot(item.Name)

		return value, nil
	case artifactDependency:
		return run.artifactDependencyValue(item)
	default:
		return "", fmt.Errorf("unknown dependency kind %q: %w", item.Kind, errPlan)
	}
}
