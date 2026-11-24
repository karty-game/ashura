// Package resource fetches bounded, checksummed prepared bytes. It does not decode
// authoring formats or create GPU resources. Admission stays with the existing consumer.
package resource

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strings"
)

const (
	maxBytes          = 128 << 20
	checksumHexLength = 64
)

// ErrInvalid marks an invalid asset descriptor or a payload that fails its
// declared size/checksum. Transport and cancellation errors retain their causes.
var ErrInvalid = errors.New("invalid prepared asset size, path or checksum")

var errAsset = ErrInvalid

// Asset identifies one immutable external prepared payload.
type Asset struct {
	ID     string
	Path   string
	Size   int
	SHA256 string
}

// Read downloads/reads exactly the descriptor's bounded prepared payload.
// Call from an I/O goroutine; admit the returned bytes on the application loop.
// Browser base is a relative same-origin URL prefix; native base is a directory.
func Read(ctx context.Context, base string, asset Asset) ([]byte, error) {
	return readAsset(ctx, base, asset, nil)
}

func readAsset(ctx context.Context, base string, asset Asset, entries []Slice) ([]byte, error) {
	if ctx == nil || !validAsset(asset) {
		return nil, fmt.Errorf("resource %q descriptor: %w", asset.ID, ErrInvalid)
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	data, err := read(ctx, base, asset.Path, asset.Size, asset.SHA256, entries)
	if err != nil {
		if canceled := ctx.Err(); canceled != nil {
			return nil, canceled
		}

		return nil, fmt.Errorf("resource %s: %w", asset.ID, err)
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	return data, nil
}

func validAsset(asset Asset) bool {
	return asset.ID != "" && fs.ValidPath(asset.Path) && !strings.ContainsAny(asset.Path, "\\%?#:\x00") &&
		asset.Size >= 1 && asset.Size <= maxBytes && validChecksum(asset.SHA256)
}

func validChecksum(sum string) bool {
	if len(sum) != checksumHexLength {
		return false
	}

	for _, char := range sum {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}

	return true
}
