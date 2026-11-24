//go:build js && wasm

package level

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"strconv"
	"syscall/js"
	"testing"

	"github.com/karty-game/ashura/internal/leveldata"
	"github.com/karty-game/ashura/pkg/engine"
	"github.com/karty-game/ashura/pkg/scene/resource"
)

func browserBundledChunkFixture(t *testing.T) (resource.Asset, map[string][]byte) {
	t.Helper()

	chunk := pendingPresentationChunk()
	page := &chunk.Pages[0]
	fields := []*Reference{
		&page.Geometry,
		&page.UV,
		&page.Maps[0],
		&page.Maps[1],
		&page.Maps[2],
		page.Materials[0].Albedo,
		page.Materials[0].SSBump,
		page.Materials[1].Albedo,
	}

	bodies := make(map[string][]byte, len(fields)+2)
	for index, field := range fields {
		contents := []byte("payload-" + strconv.Itoa(index))
		if index == 0 {
			contents = bytes.Repeat([]byte{'g'}, 37)
		}

		name := "entry-" + strconv.Itoa(index) + ".bin"

		ref, err := leveldata.Reference(name, contents)
		if err != nil {
			t.Fatal(err)
		}

		*field = ref
		bodies[name] = contents
	}

	owned, err := leveldata.ChunkOwnedReferences(chunk)
	if err != nil {
		t.Fatal(err)
	}

	var encoded bytes.Buffer

	writer := zip.NewWriter(&encoded)
	for _, ref := range owned {
		entry, createErr := writer.CreateHeader(&zip.FileHeader{Name: ref.SHA256, Method: zip.Store})
		if createErr != nil {
			t.Fatal(createErr)
		}

		if _, writeErr := entry.Write(bodies[ref.Path]); writeErr != nil {
			t.Fatal(writeErr)
		}

		delete(bodies, ref.Path)
	}

	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}

	data := encoded.Bytes()

	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}

	ref, err := leveldata.Reference("chunk.zip", data)
	if err != nil {
		t.Fatal(err)
	}

	chunk.Bundle = &ChunkBundle{File: ref}
	for _, entry := range archive.File {
		offset, entryErr := entry.DataOffset()
		if entryErr != nil {
			t.Fatal(entryErr)
		}

		chunk.Bundle.Entries = append(chunk.Bundle.Entries, BundleEntry{SHA256: entry.Name, Offset: offset})
	}

	manifest, err := leveldata.Encode(
		Manifest{
			Schema:        leveldata.Schema8,
			Source:        "fixture.scene.yaml",
			ChunkSize:     [3]float32{1, 1, 1},
			TexelsPerUnit: 1,
			Chunks:        []Chunk{chunk},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	descriptor, err := leveldata.Reference("manifest.json", manifest)
	if err != nil {
		t.Fatal(err)
	}

	bodies[ref.Path], bodies[descriptor.Path] = data, manifest

	return resource.Asset{ID: "bundle-fixture", Path: descriptor.Path, Size: descriptor.Size, SHA256: descriptor.SHA256}, bodies
}

func testBundleEntryDigestCloseCallback(t *testing.T, appClose bool) {
	t.Helper()
	descriptor, bodies := browserBundledChunkFixture(t)

	payloads := js.Global().Get("Object").New()
	for name, data := range bodies {
		value := js.Global().Get("Uint8Array").New(len(data))
		js.CopyBytesToJS(value, data)
		payloads.Set(name, value)
	}

	subtle := js.Global().Get("crypto").Get("subtle")
	oldDigest, oldFetch := subtle.Get("digest"), js.Global().Get("fetch")
	fixture := js.Global().Get("Function").New("bodies", "nativeDigest", "subtle", "archiveSize", `
const fixture = {entryStarted:false, entrySettled:false, sharedBuffer:false, count:0};
let archive;
fixture.fetch = (url) => {
    const name = String(url).slice('/fixture/'.length);
    const data = bodies[name];
    if (!data) throw new Error('unexpected unbundled request: '+name);
    fixture.count++;
    let done = false;
    return Promise.resolve({ok:true, body:{getReader:()=>({
        read:()=>{ if (done) return Promise.resolve({done:true}); done=true; return Promise.resolve({done:false,value:data}); },
        releaseLock:()=>{}
    })}});
};
fixture.digest = (algorithm, data) => {
    if (data.byteLength === archiveSize) archive = data;
    if (data.byteLength === 37) {
        fixture.entryStarted = true;
        fixture.sharedBuffer = data.buffer === archive?.buffer && data.byteOffset > archive.byteOffset;
        return new Promise(resolve=>{fixture.release=()=>resolve(new Uint8Array(32).buffer)}).finally(()=>{fixture.entrySettled=true});
    }
    return nativeDigest.call(subtle,algorithm,data);
};
return fixture;
`).Invoke(payloads, oldDigest, subtle, len(bodies["chunk.zip"]))
	js.Global().Set("fetch", fixture.Get("fetch"))

	subtle.Set("digest", fixture.Get("digest"))
	defer js.Global().Set("fetch", oldFetch)
	defer subtle.Set("digest", oldDigest)

	app := engine.NewApp(engine.AppConfig{})
	defer func() { _ = app.Close() }()

	state, err := Open(
		t.Context(),
		app,
		engine.NewWorld(),
		OpenOptions{Variants: TextureVariants{BC: descriptor, Mobile: descriptor}, Base: "/fixture", Select: WholeScene},
	)
	if err != nil {
		t.Fatal(err)
	}

	ctx := state.ctx

	var readErr error

	state.reads.Go(func() {
		var manifest Manifest

		manifest, readErr = Read(ctx, "/fixture", descriptor)
		if readErr == nil {
			_, readErr = ReadChunk(ctx, "/fixture", manifest.Chunks[0])
		}
	})
	waitPresentationFixture(t, fixture, "entryStarted")

	if !fixture.Get("sharedBuffer").Bool() {
		t.Fatal("entry checksum copied the archive view into a separate JavaScript buffer")
	}

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

	if err = <-returned; err != nil {
		t.Fatal(err)
	}

	if !errors.Is(readErr, context.Canceled) || !state.Status().Closed {
		t.Fatal("close did not cancel inner bundle integrity work", readErr, state.Status())
	}

	if fixture.Get("count").Int() != 5 {
		t.Fatal("bundle used loose geometry/map requests", fixture.Get("count"))
	}

	fixture.Get("release").Invoke()
	waitPresentationFixture(t, fixture, "entrySettled")

	if err = state.Close(); err != nil {
		t.Fatal(err)
	}
}
