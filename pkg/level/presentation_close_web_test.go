//go:build js && wasm

//nolint:paralleltest // Owns the browser-global Fetch fixture.
package level

import (
	"context"
	"errors"
	"strings"
	"syscall/js"
	"testing"
	"time"

	"github.com/karty-game/karty-webgpu/pkg/engine"
	"github.com/karty-game/karty-webgpu/pkg/scene/resource"
)

func TestPresentationCloseInsideJSCallback(t *testing.T) {
	for _, appClose := range []bool{false, true} {
		name := "Presentation.Close"
		if appClose {
			name = "App.Close"
		}

		t.Run(name, func(t *testing.T) { testPresentationCloseCallback(t, appClose) })
	}
}

func testPresentationCloseCallback(t *testing.T, appClose bool) {
	t.Helper()

	fixture := js.Global().Get("Function").New(`
const fixture = {started:false, settled:false};
fixture.fetch = (url, options) => new Promise((resolve,reject) => {
    fixture.started = true;
    options.signal.addEventListener('abort', () => reject(new Error('aborted')), {once:true});
}).finally(() => {fixture.settled = true});
return fixture;
`).Invoke()
	oldFetch := js.Global().Get("fetch")

	js.Global().Set("fetch", fixture.Get("fetch"))
	defer js.Global().Set("fetch", oldFetch)

	app := engine.NewApp(engine.AppConfig{})
	defer func() { _ = app.Close() }()

	asset := resource.Asset{ID: "pending", Path: "pending.json", Size: 1, SHA256: strings.Repeat("0", 64)}

	state, err := Open(t.Context(), app, engine.NewWorld(), OpenOptions{
		Variants: TextureVariants{BC: asset, Mobile: asset}, Base: "/fixture", Select: WholeScene,
	})
	if err != nil {
		t.Fatal(err)
	}
	// This is the same read ownership as startManifest, without requiring a GPU
	// compression enum merely to exercise the public Close/App.Close boundary.
	ctx := state.ctx

	var readErr error

	state.reads.Go(func() { _, readErr = resource.Read(ctx, "/fixture", asset) })
	waitPresentationFixture(t, fixture, "started")

	returned := make(chan error, 1)

	callback := js.FuncOf(func(js.Value, []js.Value) any {
		if appClose {
			returned <- app.Close()
		} else {
			returned <- state.Close()
		}

		return nil
	})
	defer callback.Release()

	js.Global().Call("setTimeout", callback, 0)

	if err := <-returned; err != nil {
		t.Fatal(err)
	}

	if !errors.Is(readErr, context.Canceled) || !state.Status().Closed || state.ctx != nil || state.loader != nil {
		t.Fatalf("close did not join canceled read and release ownership: error=%v status=%+v", readErr, state.Status())
	}

	if err := state.Close(); err != nil {
		t.Fatal("repeated close", err)
	}

	waitPresentationFixture(t, fixture, "settled")
}

func waitPresentationFixture(t *testing.T, fixture js.Value, field string) {
	t.Helper()

	deadline := time.Now().Add(time.Second)
	for !fixture.Get(field).Bool() {
		if time.Now().After(deadline) {
			t.Fatalf("resource Promise did not reach %s", field)
		}

		time.Sleep(time.Millisecond)
	}
}
