//go:build !js

package assetbuild

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

var errLockBusy = errors.New("asset build lock is held")

func acquireLock(ctx context.Context, project string) (func(), error) {
	full, err := confined(project, lockName)
	if err != nil {
		return nil, err
	}

	return acquireFileLock(ctx, full)
}

func acquireFileLock(ctx context.Context, full string) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
		return nil, fmt.Errorf("create asset lock directory: %w", err)
	}

	//nolint:gosec // The lock path is fixed beneath the confined project's real WebGPU state directory.
	file, err := os.OpenFile(full, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open asset build lock: %w", err)
	}

	const retryInterval = 25 * time.Millisecond

	for {
		if err := ctx.Err(); err != nil {
			_ = file.Close()

			return nil, err
		}

		err := tryLock(file)
		if err == nil {
			return func() { _ = file.Close() }, nil
		}

		if !errors.Is(err, errLockBusy) {
			_ = file.Close()

			return nil, fmt.Errorf("acquire asset build lock: %w", err)
		}

		timer := time.NewTimer(retryInterval)
		select {
		case <-ctx.Done():
			timer.Stop()

			_ = file.Close()

			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}
