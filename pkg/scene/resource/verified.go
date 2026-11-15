package resource

import (
	"context"
	"encoding/hex"

	"github.com/karty-game/karty-webgpu/internal/resourcedata"
)

// Payload is an immutable, already verified resource. Its bytes cannot be
// accessed or mutated through the public API. The zero value is not verified.
type Payload = resourcedata.Payload

// ReadVerified performs the same complete admission as Read, retaining its
// private result for a consumer that can reuse that checksum and ownership.
// It adds no payload hash, scan, copy, preparation or GPU work.
func ReadVerified(ctx context.Context, base string, asset Asset) (*Payload, error) {
	data, err := Read(ctx, base, asset)
	if err != nil {
		return nil, err
	}

	var digest [32]byte
	if _, err := hex.Decode(digest[:], []byte(asset.SHA256)); err != nil {
		return nil, err
	}

	return resourcedata.OwnVerified(data, digest), nil
}
