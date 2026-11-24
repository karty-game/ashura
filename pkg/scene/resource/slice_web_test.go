//go:build js && wasm

//nolint:paralleltest // Fixtures replace browser-global Fetch and WebCrypto.
package resource

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"syscall/js"
	"testing"

	"github.com/karty-game/ashura/internal/resourcedata"
)

func TestBrowserSliceRetainsVerifiedBytesWithoutEntryFetch(t *testing.T) {
	data := []byte("prefix:prepared entry:suffix")
	mockStream(t, [][]byte{data}, nil)

	parent, err := ReadVerified(t.Context(), "/bundle", browserAsset(data))
	if err != nil {
		t.Fatal(err)
	}

	entry := []byte("prepared entry")

	actual, err := SliceVerified(t.Context(), parent, 7, browserAsset(entry))
	if err != nil || !bytes.Equal(resourcedata.Bytes(actual), entry) ||
		resourcedata.Digest(actual) != sha256.Sum256(entry) ||
		&resourcedata.Bytes(actual)[0] != &resourcedata.Bytes(parent)[7] {
		t.Fatal("browser entry was not an immutable verified view", err)
	}

	ref := browserAsset(entry)

	ref.SHA256 = browserAsset([]byte("wrong content")).SHA256
	if actual, err := SliceVerified(t.Context(), parent, 7, ref); !errors.Is(err, ErrInvalid) || actual != nil {
		t.Fatal("outer integrity bypassed entry integrity", err)
	}
}

func TestBrowserReadVerifiedSlicesAdmitsWholeBatch(t *testing.T) {
	data := []byte("header:first:second")
	asset := browserAsset(data)
	first, second := []byte("first"), []byte("second")
	entries := []Slice{{Offset: 7, Asset: browserAsset(first)}, {Offset: 13, Asset: browserAsset(second)}}

	mockStream(t, [][]byte{data[:10], data[10:]}, nil)

	parent, result, err := ReadVerifiedSlices(t.Context(), "/bundle", asset, entries)
	if err != nil || len(result) != len(entries) {
		t.Fatal("valid archive and entries rejected", err)
	}

	backing := resourcedata.Bytes(parent)

	for index, payload := range result {
		entry := entries[index]

		view := resourcedata.Bytes(payload)
		if len(view) != entry.Asset.Size || cap(view) != len(view) || &view[0] != &backing[entry.Offset] ||
			resourcedata.Digest(payload) != sha256.Sum256(view) {
			t.Fatal("slot is not an immutable, verified archive view", index)
		}
	}

	mockStream(t, [][]byte{data}, nil)

	entries[1].Asset.SHA256 = entries[0].Asset.SHA256
	if parent, result, err = ReadVerifiedSlices(t.Context(), "/bundle", asset, entries); !errors.Is(err, ErrInvalid) ||
		parent != nil || result != nil {
		t.Fatal("bad entry exposed partial payloads", err)
	}
}

func TestBrowserSliceCancellationDuringDigest(t *testing.T) {
	data := []byte("entry")
	mockStream(t, [][]byte{data}, nil)

	parent, err := ReadVerified(t.Context(), "/bundle", browserAsset(data))
	if err != nil {
		t.Fatal(err)
	}

	subtle := js.Global().Get("crypto").Get("subtle")

	original := subtle.Get("digest")
	defer subtle.Set("digest", original)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	var resolve js.Value

	executor := js.FuncOf(func(_ js.Value, args []js.Value) any {
		resolve = args[0]

		return nil
	})
	defer executor.Release()

	digest := js.FuncOf(func(_ js.Value, _ []js.Value) any {
		promise := js.Global().Get("Promise").New(executor)

		cancel()

		return promise
	})
	defer digest.Release()

	subtle.Set("digest", digest)

	if actual, err := SliceVerified(ctx, parent, 0, browserAsset(data)); !errors.Is(err, context.Canceled) || actual != nil {
		t.Fatal("canceled digest produced an entry", actual, err)
	}

	// The native digest may settle after cancellation; its revoked Go callbacks
	// must remain detached and the derived rejection must be consumed.
	sum := sha256.Sum256(data)
	value := js.Global().Get("Uint8Array").New(len(sum))
	js.CopyBytesToJS(value, sum[:])
	resolve.Invoke(value.Get("buffer"))
}
