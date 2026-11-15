//go:build !js

package resource

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestReadVerifiesSizeChecksumCancellationAndConfinement(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	data := []byte("prepared payload")
	if err := os.WriteFile(filepath.Join(root, "asset.bin"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	asset := Asset{ID: "asset", Path: "asset.bin", Size: len(data), SHA256: fmt.Sprintf("%x", sha256.Sum256(data))}

	result, err := Read(context.Background(), root, asset)
	if err != nil || string(result) != string(data) {
		t.Fatalf("read %q: %v", result, err)
	}

	for _, mutate := range []func(*Asset){
		func(a *Asset) { a.Size-- }, func(a *Asset) { a.Size++ }, func(a *Asset) { a.SHA256 = fmt.Sprintf("%064d", 0) },
		func(a *Asset) { a.Path = "../asset.bin" }, func(a *Asset) { a.Path = "/etc/passwd" }, func(a *Asset) { a.Size = maxBytes + 1 },
		func(a *Asset) { a.Path = "..\\asset.bin" }, func(a *Asset) { a.Path = "%2e%2e/asset.bin" },
		func(a *Asset) { a.Path = "asset.bin?other" }, func(a *Asset) { a.Path = "asset.bin#other" },
	} {
		invalid := asset
		mutate(&invalid)

		if _, err := Read(context.Background(), root, invalid); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid descriptor/payload must return ErrInvalid for %+v: %v", invalid, err)
		}

		if _, err := ReadVerified(context.Background(), root, invalid); err == nil {
			t.Fatalf("verified path accepted %+v", invalid)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := Read(ctx, root, asset); !errors.Is(err, context.Canceled) || errors.Is(err, ErrInvalid) {
		t.Fatalf("cancellation must retain its cause, not become invalid data: %v", err)
	}

	if _, err := ReadVerified(ctx, root, asset); err == nil {
		t.Fatal("canceled verified read succeeded")
	}

	outside := filepath.Join(t.TempDir(), "outside.bin")
	if err := os.WriteFile(outside, data, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.Symlink(outside, filepath.Join(root, "escape.bin")); err != nil {
		t.Fatal(err)
	}

	asset.Path = "escape.bin"
	if _, err := Read(context.Background(), root, asset); err == nil {
		t.Fatal("followed symlink outside resource root")
	}

	if _, err := ReadVerified(context.Background(), root, asset); err == nil {
		t.Fatal("verified read followed symlink outside resource root")
	}
}
