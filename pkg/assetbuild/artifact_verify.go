//go:build !js

package assetbuild

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"runtime"
	"sync"
)

// Admission needs a digest and a byte count, not an owned payload allocation.
// Streaming also keeps parallel admission scratch independent of artifact size.
func (run *runner) artifactMatches(artifact Artifact) (bool, error) {
	if !validArtifact(artifact) {
		return false, errPlan
	}

	full, err := confined(run.cache, "artifacts/"+artifact.SHA256)
	if err != nil {
		return false, err
	}

	//nolint:gosec // The cache path and every component were confined above, as for readCache.
	file, err := os.Open(full)
	if err != nil {
		return false, fmt.Errorf("read asset %s: %w", full, err)
	}
	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil {
		return false, fmt.Errorf("inspect asset: %w", err)
	}

	if !info.Mode().IsRegular() || info.Size() > maxArtifactBytes {
		return false, fmt.Errorf("asset must be a regular file no larger than %d bytes: %w", maxArtifactBytes, errPath)
	}

	if info.Size() != int64(artifact.Size) {
		return false, nil
	}

	return artifactContentMatches(file, artifact)
}

func artifactContentMatches(reader io.Reader, artifact Artifact) (bool, error) {
	if !validArtifact(artifact) {
		return false, errPlan
	}

	digest := sha256.New()
	// The extra byte detects growth after Stat; short reads and all non-EOF
	// failures remain failures. io.Copy uses bounded streaming scratch.
	count, err := io.Copy(digest, io.LimitReader(reader, int64(artifact.Size)+1))
	if err != nil {
		return false, fmt.Errorf("read asset: %w", err)
	}

	return count == int64(artifact.Size) && hex.EncodeToString(digest.Sum(nil)) == artifact.SHA256, nil
}

// Every declared payload is admitted before Run. Work and errors retain the
// canonical roster order; only independent file I/O/checksum computation runs
// concurrently. Context dependency maps are updated after the workers join.
func (run *runner) admitDeclaredArtifacts(ctx context.Context, action *Context) error {
	artifacts := action.action.Artifacts
	if len(artifacts) == 0 {
		return ctx.Err()
	}

	errors := make([]error, len(artifacts))

	count := min(runtime.GOMAXPROCS(0), len(artifacts))
	if count == 1 {
		for index, artifact := range artifacts {
			errors[index] = run.admitArtifact(ctx, artifact)
		}
	} else {
		jobs := make(chan int)

		var workers sync.WaitGroup
		for range count {
			workers.Go(func() {
				for index := range jobs {
					errors[index] = run.admitArtifact(ctx, artifacts[index])
				}
			})
		}

		for index := range artifacts {
			jobs <- index
		}

		close(jobs)
		workers.Wait()
	}

	for index, err := range errors {
		if err != nil {
			return err
		}

		if err = action.trackArtifact(artifacts[index]); err != nil {
			return err
		}
	}

	return ctx.Err()
}

func (run *runner) admitArtifact(ctx context.Context, artifact Artifact) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	matches, err := run.artifactMatches(artifact)
	if err != nil {
		return err
	}

	if !matches {
		return fmt.Errorf("auxiliary artifact checksum/size mismatch: %w", errPlan)
	}

	return ctx.Err()
}
