//go:build !js

package resource

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
)

func verifySliceChecksum(ctx context.Context, data []byte, expected string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != expected {
		return ErrInvalid
	}

	return ctx.Err()
}
