//go:build js && wasm

package resource

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"syscall/js"
)

var errFetch = errors.New("browser resource request failed")

type promiseResult struct {
	value js.Value
	err   error
}

func await(ctx context.Context, promise js.Value) (js.Value, error) {
	completed := make(chan promiseResult, 1)
	success := js.FuncOf(func(_ js.Value, args []js.Value) any {
		completed <- promiseResult{value: args[0]}

		return nil
	})
	failure := js.FuncOf(func(_ js.Value, args []js.Value) any {
		completed <- promiseResult{err: fmt.Errorf("%w: %s", errFetch, args[0].String())}

		return nil
	})

	defer success.Release()
	defer failure.Release()

	// Promise reactions cannot be removed. Native revocable guards detach the Go
	// callbacks on cancellation, even if the Promise settles after WASM exits.
	// No source evaluation, application-owned work or cleanup goroutine is needed.
	proxy := js.Global().Get("Proxy")
	handler := js.Global().Get("Object").New()

	successGuard := proxy.Call("revocable", success.Value, handler)
	defer successGuard.Call("revoke")

	failureGuard := proxy.Call("revocable", failure.Value, handler)
	defer failureGuard.Call("revoke")

	// A late reaction to a revoked proxy rejects only this derived Promise. Consume
	// that rejection with a native function so cancellation cannot call back into Go.
	promise.Call("then", successGuard.Get("proxy"), failureGuard.Get("proxy")).Call("catch", js.Global().Get("Boolean"))

	select {
	case result := <-completed:
		if err := ctx.Err(); err != nil {
			return js.Undefined(), err
		}

		return result.value, result.err
	case <-ctx.Done():
		return js.Undefined(), ctx.Err()
	}
}

func read(ctx context.Context, base, name string, size int, expectedSHA string) ([]byte, error) {
	controller := js.Global().Get("AbortController").New()
	defer controller.Call("abort")

	finished := make(chan struct{})
	defer close(finished)

	go func() {
		select {
		case <-ctx.Done():
			controller.Call("abort")
		case <-finished:
		}
	}()

	url := strings.TrimSuffix(base, "/") + "/" + name

	response, err := await(
		ctx,
		js.Global().Call("fetch", url, map[string]any{"signal": controller.Get("signal"), "credentials": "same-origin"}),
	)
	if err != nil {
		return nil, err
	}

	if !response.Get("ok").Bool() {
		controller.Call("abort")

		return nil, fmt.Errorf("%w: HTTP %d", errFetch, response.Get("status").Int())
	}

	body := response.Get("body")
	if body.IsNull() || body.IsUndefined() {
		return nil, errAsset
	}

	reader := body.Call("getReader")
	defer reader.Call("releaseLock")

	data, err := readStream(ctx, reader, size)
	if err != nil {
		return nil, err
	}

	if err := verifyBrowserChecksum(ctx, data, expectedSHA); err != nil {
		return nil, err
	}

	// Keep unverified payloads outside Go memory. WebCrypto hashes asynchronously;
	// the only full payload copy into WASM happens after exact integrity admission.
	result := make([]byte, size)
	if js.CopyBytesToGo(result, data) != size {
		return nil, errAsset
	}

	return result, nil
}

func readStream(ctx context.Context, reader js.Value, size int) (js.Value, error) {
	data := js.Global().Get("Uint8Array").New(size)
	offset := 0

	for {
		chunk, err := await(ctx, reader.Call("read"))
		if err != nil {
			return js.Undefined(), err
		}

		if chunk.Get("done").Bool() {
			if offset != size {
				return js.Undefined(), errAsset
			}

			return data, nil
		}

		value := chunk.Get("value")

		length := value.Get("byteLength").Int()
		if length > size-offset {
			return js.Undefined(), errAsset
		}

		data.Call("set", value, offset)
		offset += length
	}
}

func verifyBrowserChecksum(ctx context.Context, data js.Value, expectedSHA string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	crypto := js.Global().Get("crypto")
	if crypto.IsUndefined() || crypto.IsNull() {
		return fmt.Errorf("%w: WebCrypto unavailable", errFetch)
	}

	subtle := crypto.Get("subtle")
	if subtle.IsUndefined() || subtle.IsNull() || subtle.Get("digest").Type() != js.TypeFunction {
		return fmt.Errorf("%w: WebCrypto SHA-256 unavailable", errFetch)
	}

	// A canceled digest cannot be aborted. await releases the read and detaches
	// its Go callbacks immediately, leaving only native Promise reactions.
	digest, err := await(ctx, subtle.Call("digest", "SHA-256", data))
	if err != nil {
		return err
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	value := js.Global().Get("Uint8Array").New(digest)

	var sum [32]byte
	if value.Get("byteLength").Int() != len(sum) || js.CopyBytesToGo(sum[:], value) != len(sum) ||
		hex.EncodeToString(sum[:]) != expectedSHA {
		return errAsset
	}

	return nil
}
