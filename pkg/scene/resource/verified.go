package resource

import (
	"context"
	"encoding/hex"

	"github.com/karty-game/ashura/internal/resourcedata"
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

	return ownVerified(data, asset.SHA256), nil
}

// ownVerified is called after descriptor and checksum admission.
func ownVerified(data []byte, checksum string) *Payload {
	var digest [32]byte

	_, _ = hex.Decode(digest[:], []byte(checksum))

	return resourcedata.OwnVerified(data, digest)
}
