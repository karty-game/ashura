//go:build !js

package resource

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func read(ctx context.Context, base, name string, size int, expectedSHA string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	root, err := os.OpenRoot(base)
	if err != nil {
		return nil, fmt.Errorf("open resource directory: %w", err)
	}

	defer func() { _ = root.Close() }()

	file, err := root.Open(filepath.FromSlash(name))
	if err != nil {
		return nil, fmt.Errorf("open prepared payload: %w", err)
	}

	defer func() { _ = file.Close() }()

	data, err := io.ReadAll(io.LimitReader(file, int64(size)+1))
	if err != nil {
		return nil, err
	}

	sum := sha256.Sum256(data)
	if len(data) != size || hex.EncodeToString(sum[:]) != expectedSHA {
		return nil, errAsset
	}

	return data, nil
}
