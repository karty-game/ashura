//go:build js && wasm

//nolint:paralleltest // The fixture replaces browser-global Fetch streams.
package resource

import (
	"bytes"
	"context"
	"crypto/sha256"
	"testing"

	"github.com/karty-game/karty-webgpu/internal/resourcedata"
)

func TestBrowserVerifiedPayloadRequiresActualIntegrityAdmission(t *testing.T) {
	data := []byte("immutable prepared model bytes")
	mockStream(t, [][]byte{data[:3], data[3:]}, nil)

	payload, err := ReadVerified(context.Background(), "/fixture", browserAsset(data))
	if err != nil || !resourcedata.Valid(payload) || !bytes.Equal(resourcedata.Bytes(payload), data) ||
		resourcedata.Digest(payload) != sha256.Sum256(data) {
		t.Fatal("verified payload does not match admitted bytes", err)
	}

	changed := append([]byte(nil), data...)
	changed[0] ^= 1
	mockStream(t, [][]byte{changed}, nil)

	if payload, err := ReadVerified(context.Background(), "/fixture", browserAsset(data)); err == nil || payload != nil {
		t.Fatal("checksum mismatch produced a verified payload")
	}
}
