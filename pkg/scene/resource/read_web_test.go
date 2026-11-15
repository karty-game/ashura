//go:build js && wasm

//nolint:paralleltest // Tests replace browser-global fetch and WebCrypto methods.
package resource

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"syscall/js"
	"testing"
)

// mockStream retains real browser promises and WebCrypto, replacing only the
// network transport so bounds and integrity failures are deterministic.
func mockStream(t *testing.T, chunks [][]byte, beforeRead func(int)) {
	t.Helper()

	oldFetch := js.Global().Get("fetch")

	var signal js.Value

	index := 0
	readChunk := js.FuncOf(func(_ js.Value, _ []js.Value) any {
		if beforeRead != nil {
			beforeRead(index)
		}

		chunk := map[string]any{"done": true}

		if index < len(chunks) {
			data := js.Global().Get("Uint8Array").New(len(chunks[index]))
			js.CopyBytesToJS(data, chunks[index])
			chunk = map[string]any{"done": false, "value": data}
			index++
		}

		return js.Global().Get("Promise").Call("resolve", chunk)
	})
	release := js.FuncOf(func(_ js.Value, _ []js.Value) any { return nil })
	getReader := js.FuncOf(func(_ js.Value, _ []js.Value) any {
		return map[string]any{"read": readChunk.Value, "releaseLock": release.Value}
	})
	fetch := js.FuncOf(func(_ js.Value, args []js.Value) any {
		signal = args[1].Get("signal")

		return js.Global().Get("Promise").Call("resolve", map[string]any{
			"ok": true, "body": map[string]any{"getReader": getReader.Value},
		})
	})
	js.Global().Set("fetch", fetch)
	t.Cleanup(func() {
		js.Global().Set("fetch", oldFetch)
		fetch.Release()
		getReader.Release()
		release.Release()
		readChunk.Release()
	})
	t.Cleanup(func() {
		if !signal.IsUndefined() && !signal.Get("aborted").Bool() {
			t.Error("request not closed")
		}
	})
}

func browserAsset(data []byte) Asset {
	sum := sha256.Sum256(data)

	return Asset{ID: "browser-fixture", Path: "prepared.bin", Size: len(data), SHA256: hex.EncodeToString(sum[:])}
}

func TestBrowserIntegrityMatchesNativeSHA256(t *testing.T) {
	data := make([]byte, 65539)
	for i := range data {
		data[i] = byte((i*131 + 17) & 255)
	}

	mockStream(t, [][]byte{data[:1], data[1:32768], data[32768:]}, nil)

	actual, err := Read(context.Background(), "/fixture", browserAsset(data))
	if err != nil || !bytes.Equal(actual, data) {
		t.Fatalf("verified bytes differ: len=%d error=%v", len(actual), err)
	}
}

func TestBrowserRejectsUnverifiedBytes(t *testing.T) {
	data := []byte("prepared payload")

	for _, name := range []string{"checksum", "oversize", "truncated", "invalid-checksum"} {
		t.Run(name, func(t *testing.T) {
			asset := browserAsset(data)
			chunks := [][]byte{data}

			switch name {
			case "checksum":
				asset.SHA256 = strings.Repeat("0", 64)
			case "oversize":
				chunks = [][]byte{data, {1}}
			case "truncated":
				chunks = [][]byte{data[:len(data)-1]}
			case "invalid-checksum":
				asset.SHA256 = strings.Repeat("g", 64)
			}

			mockStream(t, chunks, nil)

			actual, err := Read(context.Background(), "/fixture", asset)
			if !errors.Is(err, errAsset) || actual != nil {
				t.Fatalf("admitted %q: %v", actual, err)
			}
		})
	}
}

func TestBrowserCancellationClosesStream(t *testing.T) {
	data := []byte("prepared payload")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockStream(t, [][]byte{data[:4], data[4:]}, func(index int) {
		if index == 1 {
			cancel()
		}
	})

	actual, err := Read(ctx, "/fixture", browserAsset(data))
	if !errors.Is(err, context.Canceled) || actual != nil {
		t.Fatalf("canceled read admitted %q: %v", actual, err)
	}
}

func TestBrowserCryptoFailsClosed(t *testing.T) {
	data := []byte("prepared payload")
	subtle := js.Global().Get("crypto").Get("subtle")

	original := subtle.Get("digest")
	defer subtle.Set("digest", original)

	reject := js.FuncOf(func(_ js.Value, _ []js.Value) any {
		return js.Global().Get("Promise").Call("reject", js.Global().Get("Error").New("fixture digest failure"))
	})
	defer reject.Release()

	for _, name := range []string{"unavailable", "rejected"} {
		t.Run(name, func(t *testing.T) {
			mockStream(t, [][]byte{data}, nil)

			if name == "unavailable" {
				subtle.Set("digest", js.Undefined())
			} else {
				subtle.Set("digest", reject)
			}

			actual, err := Read(context.Background(), "/fixture", browserAsset(data))
			if err == nil || actual != nil {
				t.Fatalf("crypto failure admitted %q: %v", actual, err)
			}
		})
	}
}

func TestBrowserCancellationRejectsSettledDigest(t *testing.T) {
	data := []byte("prepared payload")
	mockStream(t, [][]byte{data}, nil)

	subtle := js.Global().Get("crypto").Get("subtle")

	original := subtle.Get("digest")
	defer subtle.Set("digest", original)

	sum := sha256.Sum256(data)
	value := js.Global().Get("Uint8Array").New(len(sum))
	js.CopyBytesToJS(value, sum[:])

	started := make(chan struct{}, 1)

	var resolve js.Value

	executor := js.FuncOf(func(_ js.Value, args []js.Value) any {
		resolve = args[0]

		return nil
	})
	defer executor.Release()

	digest := js.FuncOf(func(_ js.Value, _ []js.Value) any {
		promise := js.Global().Get("Promise").New(executor)

		started <- struct{}{}

		return promise
	})
	defer digest.Release()

	subtle.Set("digest", digest)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	settled := false

	go func() {
		<-started
		cancel()
		// WebCrypto has no cancellation API; its completion must still be consumed.
		settled = true

		resolve.Invoke(value.Get("buffer"))
	}()

	actual, err := Read(ctx, "/fixture", browserAsset(data))
	if !settled || !errors.Is(err, context.Canceled) || actual != nil {
		t.Fatalf("digest cancellation admitted %q before settlement=%v: %v", actual, settled, err)
	}
}
