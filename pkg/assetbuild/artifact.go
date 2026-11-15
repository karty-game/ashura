//go:build !js

package assetbuild

import (
	"cmp"
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
)

const artifactDependency = "artifact"

func canonicalArtifacts(artifacts []Artifact) ([]Artifact, error) {
	for _, artifact := range artifacts {
		if !validArtifact(artifact) {
			return nil, fmt.Errorf("action has invalid declared artifact: %w", errPlan)
		}
	}

	result := slices.Clone(artifacts)
	slices.SortFunc(result, func(left, right Artifact) int {
		if order := cmp.Compare(left.SHA256, right.SHA256); order != 0 {
			return order
		}

		return cmp.Compare(left.Size, right.Size)
	})

	result = slices.Compact(result)
	if len(result) > maxPlanSteps {
		return nil, fmt.Errorf("action artifact roster exceeds bounds: %w", errPlan)
	}

	return result, nil
}

// StoreArtifact writes one bounded auxiliary payload and tracks its identity as
// an action dependency. Missing or corrupt payloads invalidate a producer's cache
// hit, causing normal processing to recreate them. Blobs are project-confined,
// addressed only by SHA256, and are not automatically published to the game.
func (ctx *Context) StoreArtifact(data []byte) (Artifact, error) {
	if ctx.expanding {
		return Artifact{}, ctx.discoveryError()
	}

	if err := ctx.Err(); err != nil {
		return Artifact{}, err
	}

	if len(data) > maxArtifactBytes {
		return Artifact{}, fmt.Errorf("auxiliary artifact exceeds byte bound: %w", errPlan)
	}

	artifact := Artifact{SHA256: digest(data), Size: len(data)}
	if err := ctx.trackArtifact(artifact); err != nil {
		return Artifact{}, err
	}

	if err := ctx.runner.writeCache("artifacts/"+artifact.SHA256, data); err != nil {
		return Artifact{}, err
	}

	return artifact, ctx.Err()
}

// ReadArtifact admits a bounded payload named by a declared upstream producer.
// A reference alone does not authorize access: one declared input must transitively
// depend on its producing action. Reads track content for ordinary action caching.
func (ctx *Context) ReadArtifact(artifact Artifact) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if !validArtifact(artifact) ||
		!slices.Contains(ctx.action.Artifacts, artifact) && !ctx.runner.artifactReachable(ctx.action.Inputs, artifact) {
		return nil, fmt.Errorf("auxiliary artifact is invalid or not declared upstream: %w", errPlan)
	}

	data, err := ctx.runner.readArtifact(artifact)
	if err != nil {
		return nil, err
	}

	if err := ctx.trackArtifact(artifact); err != nil {
		return nil, err
	}

	return data, ctx.Err()
}

func validArtifact(artifact Artifact) bool {
	return validHash(artifact.SHA256) && artifact.Size >= 0 && artifact.Size <= maxArtifactBytes
}

func (ctx *Context) trackArtifact(artifact Artifact) error {
	if ctx.artifacts == nil {
		ctx.artifacts = make(map[Artifact]bool)
	}

	if !ctx.artifacts[artifact] && len(ctx.artifacts) >= maxPlanSteps {
		return fmt.Errorf("action auxiliary artifact count exceeds bound: %w", errPlan)
	}

	ctx.artifacts[artifact] = true
	ctx.deps = append(ctx.deps, dependency{Kind: artifactDependency, Name: artifact.SHA256, Value: strconv.Itoa(artifact.Size)})

	return nil
}

func dependencyArtifact(item dependency) (Artifact, bool) {
	size, err := strconv.Atoi(item.Value)
	artifact := Artifact{SHA256: item.Name, Size: size}

	return artifact, err == nil && strconv.Itoa(size) == item.Value && validArtifact(artifact)
}

func (run *runner) readArtifact(artifact Artifact) ([]byte, error) {
	data, err := run.readCache("artifacts/" + artifact.SHA256)
	if err != nil {
		return nil, err
	}

	if len(data) != artifact.Size || digest(data) != artifact.SHA256 {
		return nil, fmt.Errorf("auxiliary artifact checksum/size mismatch: %w", errPlan)
	}

	return data, nil
}

func (run *runner) artifactDependencyValue(item dependency) (string, error) {
	artifact, ok := dependencyArtifact(item)
	if !ok {
		return "", errPlan
	}

	matches, err := run.artifactMatches(artifact)
	if errors.Is(err, os.ErrNotExist) {
		return missingDependency, nil
	}

	if err != nil {
		return "", err
	}

	if !matches {
		return "corrupt", nil
	}

	return item.Value, nil
}

func (run *runner) rememberArtifacts(action string, deps []dependency) {
	artifacts := make(map[Artifact]bool)

	for _, item := range deps {
		if item.Kind == artifactDependency {
			artifact, ok := dependencyArtifact(item)
			if ok {
				artifacts[artifact] = true
			}
		}
	}

	run.artifacts[action] = artifacts
}

func (run *runner) artifactReachable(inputs []Ref, artifact Artifact) bool {
	seen := make(map[string]bool)

	queue := append([]Ref(nil), inputs...)
	for len(queue) > 0 {
		ref := queue[len(queue)-1]
		queue = queue[:len(queue)-1]

		if seen[ref.Action] {
			continue
		}

		seen[ref.Action] = true
		if run.artifacts[ref.Action][artifact] {
			return true
		}

		queue = append(queue, run.actions[ref.Action].Inputs...)
	}

	return false
}

func declaredArtifactsPresent(action Action, deps []dependency) bool {
	if len(action.Artifacts) == 0 {
		return true
	}

	artifacts := make(map[Artifact]bool)

	for _, item := range deps {
		if item.Kind == artifactDependency {
			artifact, ok := dependencyArtifact(item)
			if ok {
				artifacts[artifact] = true
			}
		}
	}

	for _, artifact := range action.Artifacts {
		if !artifacts[artifact] {
			return false
		}
	}

	return true
}
