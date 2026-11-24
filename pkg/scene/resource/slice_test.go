//go:build !js

package resource

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/karty-game/ashura/internal/resourcedata"
)

func sliceAsset(data []byte) Asset {
	digest := sha256.Sum256(data)

	return Asset{ID: "entry", Path: "entry.bin", Size: len(data), SHA256: hex.EncodeToString(digest[:])}
}

func readSliceParent(t *testing.T, data []byte) *Payload {
	t.Helper()
	root := t.TempDir()
	asset := sliceAsset(data)

	if err := os.WriteFile(filepath.Join(root, asset.Path), data, 0600); err != nil {
		t.Fatal(err)
	}

	parent, err := ReadVerified(t.Context(), root, asset)
	if err != nil {
		t.Fatal(err)
	}

	return parent
}

func TestSliceVerifiedRetainsImmutableParentView(t *testing.T) {
	t.Parallel()

	data := []byte("header:prepared vertex bytes:footer")
	entry := []byte("prepared vertex bytes")
	payload := func() *Payload {
		parent := readSliceParent(t, data)

		payload, err := SliceVerified(t.Context(), parent, 7, sliceAsset(entry))
		if err != nil || !resourcedata.Valid(payload) || !bytes.Equal(resourcedata.Bytes(payload), entry) {
			t.Fatal("verified entry differs", payload, err)
		}

		view, backing := resourcedata.Bytes(payload), resourcedata.Bytes(parent)
		if &view[0] != &backing[7] || cap(view) != len(view) || resourcedata.Digest(payload) != sha256.Sum256(entry) {
			t.Fatal("entry was copied, was not capacity-clamped, or lost integrity identity")
		}

		return payload
	}()

	runtime.GC()

	if !bytes.Equal(resourcedata.Bytes(payload), entry) {
		t.Fatal("entry lost parent backing storage")
	}
}

func TestSliceVerifiedRejectsTamperedBytesAndBounds(t *testing.T) {
	t.Parallel()

	entry := []byte("entry bytes")
	parent := readSliceParent(t, append([]byte("prefix"), entry...))
	asset := sliceAsset(entry)

	for _, offset := range []int64{-1, math.MaxInt64, 7} {
		if result, err := SliceVerified(t.Context(), parent, offset, asset); !errors.Is(err, ErrInvalid) || result != nil {
			t.Fatal("invalid entry bound admitted", offset, result, err)
		}
	}

	for name, change := range map[string]func(*Asset){
		"size":   func(ref *Asset) { ref.Size-- },
		"path":   func(ref *Asset) { ref.Path = "../entry" },
		"digest": func(ref *Asset) { ref.SHA256 = sliceAsset([]byte("different")).SHA256 },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			ref := asset
			change(&ref)

			if result, err := SliceVerified(t.Context(), parent, 6, ref); !errors.Is(err, ErrInvalid) || result != nil {
				t.Fatal("unverified entry admitted", result, err)
			}
		})
	}

	for _, invalid := range []*Payload{nil, {}} {
		if result, err := SliceVerified(t.Context(), invalid, 0, asset); !errors.Is(err, ErrInvalid) || result != nil {
			t.Fatal("unverified parent admitted", result, err)
		}
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if result, err := SliceVerified(ctx, parent, 6, asset); !errors.Is(err, context.Canceled) || result != nil {
		t.Fatal("cancellation lost its cause", result, err)
	}
}
