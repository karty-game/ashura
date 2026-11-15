//go:build js && wasm

//nolint:paralleltest // These checks replace browser-global Fetch and WebCrypto methods.
package resource

import (
	"context"
	"crypto/sha256"
	"errors"
	"syscall/js"
	"testing"
	"time"
)

// The fixture retains actual Promise/microtask scheduling. Pending fetch/stream
// work rejects on abort; the uncancelable digest settles only after Close returns.
const cancellationFixture = `
const fixture = {started:false, settled:false, aborted:false, released:false};
let signal, offset = 0;
function pending(value, abortable) {
    return new Promise((resolve,reject) => {
        fixture.finish = () => resolve(value);
        fixture.started = true;
        if (abortable) signal.addEventListener('abort', () => {
            fixture.aborted = true;
            reject(new Error('fixture aborted'));
        }, {once:true});
    }).finally(() => {fixture.settled = true});
}
const reader = {
    read() {
        if (phase === 'stream') return pending({done:true}, true);
        return Promise.resolve(offset++ ? {done:true} : {done:false, value:data});
    },
    releaseLock() {fixture.released = true}
};
const response = {ok:true, body:{getReader() {return reader}}};
fixture.fetch = (url, options) => {
    signal = options.signal;
    signal.addEventListener('abort', () => {fixture.aborted = true}, {once:true});
    return phase === 'fetch' ? pending(response, true) : Promise.resolve(response);
};
fixture.digest = () => pending(sum.buffer, false);
return fixture;
`

type canceledRead struct {
	data []byte
	err  error
}

func TestBrowserCanceledContextStartsNoFetch(t *testing.T) {
	data := []byte("prepared cancellation fixture")
	fixture := installCancellationFixture(t, "fetch", data)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	actual, err := Read(ctx, "/fixture", browserAsset(data))
	if actual != nil || !errors.Is(err, context.Canceled) || fixture.Get("started").Bool() {
		t.Fatal("already canceled request started transport or admitted data", actual, err)
	}
}

func TestBrowserCanceledAwaitDetachesCallbacks(t *testing.T) {
	fixture := installCancellationFixture(t, "fetch", []byte("fixture"))
	controller := js.Global().Get("AbortController").New()
	promise := fixture.Get("fetch").Invoke("/fixture", map[string]any{"signal": controller.Get("signal")})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := await(ctx, promise); !errors.Is(err, context.Canceled) {
		t.Fatal("already canceled Promise wait did not detach", err)
	}

	settleCancellationFixture(t, fixture)
}

func TestBrowserCancellationReturnsInsideJSCallback(t *testing.T) {
	for _, phase := range []string{"fetch", "stream", "digest"} {
		t.Run(phase, func(t *testing.T) {
			data := []byte("prepared cancellation fixture")
			fixture := installCancellationFixture(t, phase, data)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			completed := make(chan canceledRead, 1)

			go func() {
				payload, err := Read(ctx, "/fixture", browserAsset(data))
				completed <- canceledRead{data: payload, err: err}
			}()

			waitCancellationFixture(t, fixture)

			var result canceledRead

			returned := make(chan struct{})

			callback := js.FuncOf(func(js.Value, []js.Value) any {
				cancel()
				// This join must finish before JS can run any abort/digest microtask.
				result = <-completed

				close(returned)

				return nil
			})
			defer callback.Release()

			js.Global().Call("setTimeout", callback, 0)
			<-returned

			if !errors.Is(result.err, context.Canceled) || result.data != nil {
				t.Fatalf("canceled %s admitted %q: %v", phase, result.data, result.err)
			}

			settleCancellationFixture(t, fixture)

			if !fixture.Get("aborted").Bool() || phase != "fetch" && !fixture.Get("released").Bool() {
				t.Fatal("cancellation did not abort/release the resource stream")
			}
		})
	}
}

func installCancellationFixture(t *testing.T, phase string, data []byte) js.Value {
	t.Helper()

	bytes := js.Global().Get("Uint8Array").New(len(data))
	js.CopyBytesToJS(bytes, data)
	sum := sha256.Sum256(data)
	digest := js.Global().Get("Uint8Array").New(len(sum))
	js.CopyBytesToJS(digest, sum[:])
	fixture := js.Global().Get("Function").New("phase", "data", "sum", cancellationFixture).Invoke(phase, bytes, digest)
	oldFetch := js.Global().Get("fetch")
	js.Global().Set("fetch", fixture.Get("fetch"))
	t.Cleanup(func() { js.Global().Set("fetch", oldFetch) })

	if phase == "digest" {
		subtle := js.Global().Get("crypto").Get("subtle")
		oldDigest := subtle.Get("digest")
		subtle.Set("digest", fixture.Get("digest"))
		t.Cleanup(func() { subtle.Set("digest", oldDigest) })
	}

	return fixture
}

func waitCancellationFixture(t *testing.T, fixture js.Value) {
	t.Helper()

	deadline := time.Now().Add(time.Second)
	for !fixture.Get("started").Bool() {
		if time.Now().After(deadline) {
			t.Fatal("pending resource phase did not start")
		}

		time.Sleep(time.Millisecond)
	}
}

func settleCancellationFixture(t *testing.T, fixture js.Value) {
	t.Helper()

	console := js.Global().Get("console")
	oldError := console.Get("error")

	var messages []string

	consoleError := js.FuncOf(func(_ js.Value, args []js.Value) any {
		for _, argument := range args {
			messages = append(messages, argument.String())
		}

		return nil
	})

	console.Set("error", consoleError)
	defer consoleError.Release()
	defer console.Set("error", oldError)

	fixture.Call("finish")
	// A timer turns the JS loop after the cancellation callback has returned and
	// drains pending Promise callbacks, including retained Go callback cleanup.
	nextTurn := js.Global().Get("Promise").New(js.Global().Get("Function").New("resolve", "setTimeout(resolve, 0)"))
	if _, err := await(context.Background(), nextTurn); err != nil {
		t.Fatal(err)
	}

	if !fixture.Get("settled").Bool() || len(messages) != 0 {
		t.Fatalf("late Promise settlement failed: settled=%v console=%v", fixture.Get("settled").Bool(), messages)
	}
}

func BenchmarkBrowserPromiseAwait(b *testing.B) {
	promise := js.Global().Get("Promise").Call("resolve", 7)

	b.ReportAllocs()

	for b.Loop() {
		value, err := await(context.Background(), promise)
		if err != nil || value.Int() != 7 {
			b.Fatal("settled resource Promise", value, err)
		}
	}
}
