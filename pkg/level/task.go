package level

import (
	"context"
	"errors"

	"github.com/karty-game/karty-webgpu/internal/leveldata"
	"github.com/karty-game/karty-webgpu/internal/texturedata"
)

// ErrLoadPending means the task has not completed all texture/page steps.
var ErrLoadPending = errors.New("level load task is not complete")

// LoadTask admits one selected chunk progressively on the App loop. Each Step
// copies at most one new texture or constructs one prepared geometry page.
// Existing resident texture handles require only bounded metadata/refcount work.
type LoadTask struct {
	loader                *Loader
	prepared              *PreparedChunk
	chunk                 *LoadedChunk
	nextTexture, nextPage int
	done                  bool
	failure               error
}

// Begin validates the residency snapshot and retains existing texture handles.
// It performs no payload copies, geometry admission, uploads or file reads.
func (loader *Loader) Begin(prepared *PreparedChunk) (*LoadTask, error) {
	if loader == nil || loader.closed || loader.app.Closed() {
		return nil, errLifetime
	}

	if prepared == nil || len(prepared.pages) == 0 {
		return nil, ErrFormat
	}

	for _, ref := range prepared.imageRefs {
		shared := loader.textures[ref.SHA256]
		if shared != nil {
			if !texturedata.Valid(shared.image) {
				return nil, errLifetime
			}

			if shared.metadata() != prepared.imageInfo[ref.SHA256] {
				return nil, leveldata.ErrFormat
			}
		} else if _, skipped := prepared.resident[ref.SHA256]; skipped {
			return nil, ErrStaleRequest
		}
	}

	chunk := &LoadedChunk{
		loader:      loader,
		description: prepared.chunk,
		instances:   make(map[*Instance]bool),
		textureSet:  make(map[string]bool),
	}
	for _, ref := range prepared.imageRefs {
		if shared := loader.textures[ref.SHA256]; shared != nil {
			chunk.retain(ref.SHA256, shared)
		}
	}

	task := &LoadTask{loader: loader, prepared: prepared, chunk: chunk}
	loader.tasks[task] = true

	return task, nil
}

// Step prepares at most one new texture (CPU ownership plus an active backend
// upload) or constructs one page. Pre-Run/legacy drivers retain first-use upload
// behavior through App.PrepareTexture; no source processing or repacking occurs.
func (task *LoadTask) Step() (bool, error) {
	if task == nil {
		return true, errLifetime
	}

	if task.done {
		return true, task.failure
	}

	if task.loader.closed || task.loader.app.Closed() {
		return task.fail(errLifetime)
	}

	for task.nextTexture < len(task.prepared.imageRefs) {
		ref := task.prepared.imageRefs[task.nextTexture]
		existing := task.loader.textures[ref.SHA256] != nil

		image, err := task.chunk.acquire(task.prepared, ref)
		if err != nil {
			return task.fail(err)
		}

		task.nextTexture++
		if !existing {
			if err = task.loader.app.PrepareTexture(image); err != nil {
				return task.fail(err)
			}

			return false, nil
		}
	}

	if task.nextPage < len(task.prepared.pages) {
		if err := task.chunk.loadPage(task.prepared.pages[task.nextPage]); err != nil {
			return task.fail(err)
		}

		task.nextPage++
	}

	if task.nextPage == len(task.prepared.pages) {
		task.done = true
		task.prepared = nil
		task.loader.chunks[task.chunk] = true
		delete(task.loader.tasks, task)

		return true, nil
	}

	return false, nil
}

// Result returns the complete chunk only after successful final page admission.
func (task *LoadTask) Result() (*LoadedChunk, error) {
	if task == nil {
		return nil, errLifetime
	}

	if !task.done {
		return nil, ErrLoadPending
	}

	if task.failure != nil {
		return nil, task.failure
	}

	return task.chunk, nil
}

// Cancel releases unpublished resources and retained handles. Completed chunks
// remain owned by the caller/Loader. Cancellation is idempotent.
func (task *LoadTask) Cancel() error {
	if task == nil || task.done && task.failure == nil {
		return nil
	}

	if !task.done {
		task.done = true
		task.failure = context.Canceled
	}

	task.prepared = nil
	if task.chunk == nil || task.chunk.closed {
		delete(task.loader.tasks, task)

		return nil
	}

	if err := task.chunk.Unload(); err != nil {
		return err
	}

	delete(task.loader.tasks, task)

	return nil
}

func (task *LoadTask) fail(failure error) (bool, error) {
	task.done = true
	task.failure = failure
	task.prepared = nil
	cleanup := task.Cancel()
	task.failure = errors.Join(task.failure, cleanup)

	return true, task.failure
}
