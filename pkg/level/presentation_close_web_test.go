//go:build js && wasm

//nolint:paralleltest // Owns the browser-global Fetch fixture.
package level

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"syscall/js"
	"testing"
	"time"

	"github.com/karty-game/ashura/internal/leveldata"
	"github.com/karty-game/ashura/pkg/engine"
	"github.com/karty-game/ashura/pkg/scene/resource"
)

func TestPresentationCloseInsideJSCallback(t *testing.T) {
	for _, appClose := range []bool{false, true} {
		name := "Presentation.Close"
		if appClose {
			name = "App.Close"
		}

		t.Run(name, func(t *testing.T) {
			t.Run("manifest", func(t *testing.T) { testPresentationCloseCallback(t, appClose, false, false) })
			t.Run("parallel-chunk", func(t *testing.T) { testPresentationCloseCallback(t, appClose, true, false) })
			t.Run("bundled-chunk", func(t *testing.T) { testPresentationCloseCallback(t, appClose, true, true) })
			t.Run("bundle-entry-digest", func(t *testing.T) { testBundleEntryDigestCloseCallback(t, appClose) })
		})
	}
}

func testPresentationCloseCallback(t *testing.T, appClose, chunkRead, bundled bool) {
	t.Helper()

	expectedReads := 1
	if chunkRead {
		expectedReads = 8
	}

	if bundled {
		expectedReads = 4 // One owned bundle plus the three distinct shared material textures.
	}

	fixture := js.Global().Get("Function").New("expected", `
const fixture = {started:false, settled:false, count:0, finished:0, aborted:0};
fixture.fetch = (url, options) => new Promise((resolve,reject) => {
    fixture.count++;
    fixture.started = fixture.count === expected;
    options.signal.addEventListener('abort', () => {
        fixture.aborted++;
        reject(new Error('aborted'));
    }, {once:true});
}).finally(() => {fixture.finished++; fixture.settled = fixture.finished === expected});
return fixture;
`).Invoke(expectedReads)
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

	state.reads.Go(func() {
		if chunkRead {
			chunk := pendingPresentationChunk()
			if bundled {
				chunk = pendingBundledPresentationChunk(chunk)
			}

			_, readErr = ReadChunk(ctx, "/fixture", chunk)
		} else {
			_, readErr = resource.Read(ctx, "/fixture", asset)
		}
	})
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

	if fixture.Get("count").Int() != expectedReads || fixture.Get("aborted").Int() != expectedReads {
		t.Fatal("close did not abort every outstanding request", fixture.Get("count"), fixture.Get("aborted"))
	}
}

func pendingBundledPresentationChunk(chunk Chunk) Chunk {
	chunk.Bundle = &leveldata.ChunkBundle{
		File: Reference{Path: "pending-bundle.zip", Size: 5, SHA256: strings.Repeat("9", 64)},
	}

	refs := make([]Reference, 2, 2+len(chunk.Pages[0].Maps))
	refs[0], refs[1] = chunk.Pages[0].Geometry, chunk.Pages[0].UV

	refs = append(refs, chunk.Pages[0].Maps[:]...)
	for index, ref := range refs {
		chunk.Bundle.Entries = append(chunk.Bundle.Entries, leveldata.BundleEntry{SHA256: ref.SHA256, Offset: int64(index)})
	}

	return chunk
}

func pendingPresentationChunk() Chunk {
	var refs [8]Reference
	for index := range refs {
		identity := strconv.Itoa(index)
		refs[index] = Reference{Path: "pending-" + identity + ".bin", Size: 1, SHA256: strings.Repeat(identity, 64)}
	}

	return Chunk{
		ID: "pending-chunk", Space: "default", Max: [3]float32{1, 1, 1},
		Pages: []Page{{
			Geometry: refs[0], UV: refs[1], Maps: [3]Reference{refs[2], refs[3], refs[4]},
			Width: 16, Height: 16, Scale: 1, Encoding: "rgbe8", Representation: RNMFit,
			Materials: []Material{
				{Name: "one", Color: [4]float32{1, 1, 1, 1}, Albedo: &refs[5], SSBump: &refs[6]},
				{Name: "two", Color: [4]float32{1, 1, 1, 1}, Albedo: &refs[7], SSBump: &refs[6]},
			},
		}},
	}
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
