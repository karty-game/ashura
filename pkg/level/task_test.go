//go:build !js

package level

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/karty-game/karty-webgpu/internal/texturedata"
	"github.com/karty-game/karty-webgpu/pkg/engine"
)

func TestResidentRequestSkipsTextureFilesAndContainerDecoding(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	description := fixtureChunk(t, root)

	initial, err := ReadChunk(t.Context(), root, description)
	if err != nil {
		t.Fatal(err)
	}

	app := engine.NewApp(engine.AppConfig{})
	defer func() { _ = app.Close() }()

	loader := NewLoader(app)

	resident, err := loader.Load(initial)
	if err != nil {
		t.Fatal(err)
	}

	request, err := loader.Request(description)
	if err != nil {
		t.Fatal(err)
	}

	if len(request.resident) != 3 {
		t.Fatal("request did not snapshot only this chunk's resident images")
	}

	refs := []Reference{description.Pages[0].Maps[0], *description.Pages[0].Materials[0].Albedo, *description.Pages[0].Materials[0].SSBump}
	for _, ref := range refs {
		if err = os.Remove(filepath.Join(root, ref.Path)); err != nil {
			t.Fatal(err)
		}
	}

	prepared, err := request.Read(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}

	if len(prepared.images) != 0 || len(prepared.files) != 2 || len(prepared.imageInfo) != 3 {
		t.Fatal("resident images were fetched or decoded again")
	}

	replacement, err := loader.Load(prepared)
	if err != nil {
		t.Fatal(err)
	}

	for _, shared := range loader.textures {
		if shared.references != 2 {
			t.Fatal("resident image reference count changed")
		}
	}

	original, next := resident.pages[0].materials[0], replacement.pages[0].materials[0]
	if original.AlbedoTexture != next.AlbedoTexture || original.SSBumpTexture != next.SSBumpTexture ||
		original.RNMLightmap.Textures != next.RNMLightmap.Textures {
		t.Fatal("resident image handles were duplicated")
	}

	if err = replacement.Unload(); err != nil {
		t.Fatal(err)
	}

	if err = resident.Unload(); err != nil {
		t.Fatal(err)
	}
}

func TestRequestEvictionRequiresExplicitRetry(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	description := fixtureChunk(t, root)

	initial, err := ReadChunk(t.Context(), root, description)
	if err != nil {
		t.Fatal(err)
	}

	app := engine.NewApp(engine.AppConfig{})
	defer func() { _ = app.Close() }()

	loader := NewLoader(app)

	resident, err := loader.Load(initial)
	if err != nil {
		t.Fatal(err)
	}

	request, err := loader.Request(description)
	if err != nil {
		t.Fatal(err)
	}

	if err = resident.Unload(); err != nil {
		t.Fatal(err)
	}

	prepared, err := request.Read(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}

	if task, err := loader.Begin(prepared); !errors.Is(err, ErrStaleRequest) || task != nil {
		t.Fatalf("eviction reused a stale handle: %v", err)
	}

	if len(loader.tasks) != 0 || len(loader.chunks) != 0 || len(loader.textures) != 0 {
		t.Fatal("stale request changed loader ownership")
	}

	retry, err := loader.Request(description)
	if err != nil {
		t.Fatal(err)
	}

	prepared, err = retry.Read(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}

	loaded, err := loader.Load(prepared)
	if err != nil {
		t.Fatal(err)
	}

	if err = loaded.Unload(); err != nil {
		t.Fatal(err)
	}
}

func TestProgressiveStepAdmitsAtMostOneNewTextureOrPage(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	description := fixtureChunk(t, root)
	description.Pages = append(description.Pages, description.Pages[0])

	prepared, err := ReadChunk(t.Context(), root, description)
	if err != nil {
		t.Fatal(err)
	}

	app := engine.NewApp(engine.AppConfig{})
	defer func() { _ = app.Close() }()

	loader := NewLoader(app)

	task, err := loader.Begin(prepared)
	if err != nil {
		t.Fatal(err)
	}

	if len(loader.textures) != 0 || len(task.chunk.pages) != 0 {
		t.Fatal("Begin performed payload work")
	}

	if _, err = task.Result(); !errors.Is(err, ErrLoadPending) {
		t.Fatal("incomplete chunk published")
	}

	steps := driveTaskWithBudget(t, loader, task)

	if steps != len(prepared.imageRefs)+len(prepared.pages) {
		t.Fatalf("step count %d does not match explicit payload units", steps)
	}

	loaded, err := task.Result()
	if err != nil {
		t.Fatal(err)
	}

	if len(loader.tasks) != 0 || !loader.chunks[loaded] || task.prepared != nil {
		t.Fatal("completed task retained staging data or pending ownership")
	}

	if done, err := task.Step(); !done || err != nil {
		t.Fatalf("completed Step not idempotent: %v", err)
	}

	if err = task.Cancel(); err != nil || loaded.closed {
		t.Fatalf("Cancel destroyed a successfully published chunk: %v", err)
	}

	synchronous, err := loader.Load(prepared)
	if err != nil {
		t.Fatal(err)
	}

	if len(synchronous.pages) != len(loaded.pages) || len(loader.textures) != 3 {
		t.Fatal("synchronous Load differs from progressive result")
	}

	for index := range loaded.pages {
		first, second := loaded.pages[index].materials[0], synchronous.pages[index].materials[0]
		if first.Color != second.Color || first.SSBumpTexture != second.SSBumpTexture || *first.RNMLightmap != *second.RNMLightmap {
			t.Fatal("material behavior changed between load APIs")
		}
	}

	if err = synchronous.Unload(); err != nil {
		t.Fatal(err)
	}

	if err = loaded.Unload(); err != nil {
		t.Fatal(err)
	}
}

func driveTaskWithBudget(t *testing.T, loader *Loader, task *LoadTask) int {
	t.Helper()

	steps := 0

	for {
		images, pages := len(loader.textures), len(task.chunk.pages)

		done, err := task.Step()
		if err != nil {
			t.Fatal(err)
		}

		steps++

		newImages, newPages := len(loader.textures)-images, len(task.chunk.pages)-pages
		if newImages > 1 || newPages > 1 || newImages+newPages > 1 {
			t.Fatal("one Step did more than one bounded payload unit")
		}

		if done {
			break
		}

		if loader.chunks[task.chunk] {
			t.Fatal("partial chunk published")
		}
	}

	return steps
}

func TestPendingLoadPinsSharedImagesAndCancellationReleasesThem(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	description := fixtureChunk(t, root)

	initial, err := ReadChunk(t.Context(), root, description)
	if err != nil {
		t.Fatal(err)
	}

	app := engine.NewApp(engine.AppConfig{})
	defer func() { _ = app.Close() }()

	loader := NewLoader(app)

	resident, err := loader.Load(initial)
	if err != nil {
		t.Fatal(err)
	}

	request, err := loader.Request(description)
	if err != nil {
		t.Fatal(err)
	}

	prepared, err := request.Read(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}

	task, err := loader.Begin(prepared)
	if err != nil {
		t.Fatal(err)
	}

	if err = resident.Unload(); err != nil {
		t.Fatal(err)
	}

	for _, shared := range loader.textures {
		if shared.references != 1 || !texturedata.Valid(shared.image) {
			t.Fatal("pending load failed to retain its shared texture")
		}
	}

	if err = task.Cancel(); err != nil {
		t.Fatal(err)
	}

	if len(loader.textures) != 0 || len(loader.tasks) != 0 || len(loader.chunks) != 0 {
		t.Fatal("pending cancellation leaked references")
	}

	if done, err := task.Step(); !done || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled task continued: %v", err)
	}

	if _, err = task.Result(); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled chunk published")
	}

	if err = task.Cancel(); err != nil {
		t.Fatal("Cancel not idempotent")
	}
}

func TestAppCloseCancelsPartiallyLoadedChunk(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	description := fixtureChunk(t, root)

	prepared, err := ReadChunk(t.Context(), root, description)
	if err != nil {
		t.Fatal(err)
	}

	app := engine.NewApp(engine.AppConfig{})
	loader := NewLoader(app)

	task, err := loader.Begin(prepared)
	if err != nil {
		t.Fatal(err)
	}

	if done, err := task.Step(); done || err != nil || len(loader.textures) != 1 {
		t.Fatalf("first payload step: %v", err)
	}

	if err = app.Close(); err != nil {
		t.Fatal(err)
	}

	if !loader.closed || len(loader.tasks) != 0 || len(loader.textures) != 0 || !task.chunk.closed || task.prepared != nil {
		t.Fatal("App close retained pending load resources")
	}

	if _, err = task.Result(); !errors.Is(err, context.Canceled) {
		t.Fatal("App close published pending chunk")
	}
}
