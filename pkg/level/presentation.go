package level

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/karty-game/karty-webgpu/pkg/engine"
	"github.com/karty-game/karty-webgpu/pkg/model"
)

type presentationArrival struct {
	manifest *Manifest
	prepared *PreparedChunk
	err      error
}

// Presentation owns explicitly selected prepared chunks and independent models.
// Its methods belong to the App loop; Close cancels and joins its I/O tasks.
type Presentation struct {
	//nolint:containedctx // One presentation owns these cancellable I/O tasks and joins them before releasing resources.
	ctx              context.Context
	cancel           context.CancelFunc
	reads            sync.WaitGroup
	app              *engine.App
	world            *engine.World
	loader           *Loader
	pending          chan presentationArrival
	independent      chan independentArrival
	options          OpenOptions
	selection        Selection
	failure          error
	closeCancel      func()
	manifest         *Manifest
	task             *LoadTask
	presenting       *Instance
	instances        []*Instance
	loaded           int
	started          bool
	reading          bool
	independentReady bool
	models           []*model.Model
	modelInstances   []independentInstance
	closed           bool
	released         bool
}

// Close removes presentations and releases owned resources after joining I/O.
// Calls emit no status notification. If external bindings cause ErrInUse,
// release those bindings and call Close again; ownership is retained until then.
func (state *Presentation) Close() (err error) {
	if state == nil || state.released {
		return nil
	}

	if !state.closed {
		state.closed = true
		state.cancel()
		state.reads.Wait()
	}

	if state.task != nil {
		err = errors.Join(err, state.task.Cancel())
	}

	for _, instance := range state.instances {
		instance.Remove()
	}

	for _, item := range state.modelInstances {
		item.instance.Remove()
	}

	for _, asset := range state.models {
		err = errors.Join(err, asset.Unload())
	}

	err = errors.Join(err, state.loader.Close())
	state.ctx, state.cancel = nil, nil
	state.options.StatusChanged = nil
	state.options.Select = nil
	state.pending, state.independent = nil, nil
	state.presenting, state.manifest = nil, nil
	state.instances, state.modelInstances = nil, nil

	state.selection = Selection{}
	if err == nil {
		state.released = true
		if state.closeCancel != nil {
			state.closeCancel()
			state.closeCancel = nil
		}

		state.task, state.models = nil, nil
		state.app, state.world, state.loader = nil, nil, nil
	}

	return err
}

// Update advances the existing loading or presentation step on the App loop.
// Register it with App.OnUpdateError; its delta is not a scheduling budget.
func (state *Presentation) Update(time.Duration) error {
	if state == nil || state.closed {
		return nil
	}

	if state.failure != nil {
		return state.failure
	}

	if err := state.ctx.Err(); err != nil {
		state.failure = err

		return err
	}

	state.failure = state.advance()

	return state.failure
}

// Status returns stored progress only; it performs no resource or world scan.
func (state *Presentation) Status() PresentationStatus {
	if state == nil || state.closed {
		return PresentationStatus{Closed: true}
	}

	return PresentationStatus{
		LoadedChunks: state.loaded, TotalChunks: len(state.selection.Chunks),
		LoadedObjects: len(state.modelInstances), TotalObjects: len(state.selection.Objects),
		Ready: state.manifest != nil && state.loaded == len(state.selection.Chunks) && state.independentReady && state.failure == nil,
	}
}

func (state *Presentation) advance() error {
	if state.closed {
		return nil
	}

	if !state.started {
		return state.startManifest()
	}

	if err := state.pollIndependent(); err != nil {
		return err
	}

	if state.presenting != nil {
		return state.presentNext()
	}

	if state.task != nil {
		return state.loadNext()
	}

	select {
	case item := <-state.pending:
		if err := state.accept(item); err != nil {
			return err
		}
	default:
	}

	if state.manifest != nil && state.loaded < len(state.selection.Chunks) && !state.reading && state.task == nil {
		return state.requestChunk()
	}

	return nil
}

func (state *Presentation) startManifest() error {
	descriptor, err := state.options.Variants.Select(state.app.CompressedAlbedoFormat())
	if err != nil {
		return err
	}

	state.started, state.reading = true, true
	state.reads.Go(func() {
		manifest, err := Read(state.ctx, state.options.Base, descriptor)
		select {
		case state.pending <- presentationArrival{manifest: &manifest, err: err}:
		case <-state.ctx.Done():
		}
	})

	return nil
}

func (state *Presentation) accept(item presentationArrival) error {
	state.reading = false

	if item.err != nil {
		return item.err
	}

	if item.manifest != nil {
		selected, err := state.options.Select(*item.manifest)
		if err != nil {
			return err
		}

		if state.closed {
			return nil
		}

		if err = validatePresentationSelection(*item.manifest, selected); err != nil {
			return err
		}

		state.selection = cloneSelection(selected)

		state.manifest = item.manifest
		if err := state.requestIndependent(); err != nil {
			return err
		}

		state.notify()
	}

	if item.prepared != nil {
		task, err := state.loader.Begin(item.prepared)
		if err != nil {
			return err
		}

		state.task = task
	}

	return nil
}

func (state *Presentation) requestChunk() error {
	request, err := state.loader.Request(state.manifest.Chunks[state.selection.Chunks[state.loaded]])
	if err != nil {
		return err
	}

	state.reading = true
	state.reads.Go(func() {
		prepared, err := request.Read(state.ctx, state.options.Base)
		select {
		case state.pending <- presentationArrival{prepared: prepared, err: err}:
		case <-state.ctx.Done():
		}
	})

	return nil
}

func (state *Presentation) loadNext() error {
	done, err := state.task.Step()
	if err != nil || !done {
		return err
	}

	chunk, err := state.task.Result()
	if err != nil {
		return err
	}

	instance, err := chunk.BeginPresentation(state.world)
	if err != nil {
		return err
	}

	state.instances = append(state.instances, instance)
	state.presenting = instance
	state.task = nil

	return nil
}

func (state *Presentation) presentNext() error {
	done, err := state.presenting.PresentNext()
	if err != nil || !done {
		return err
	}

	state.presenting = nil
	state.loaded++
	state.notify()

	return nil
}

func (state *Presentation) notify() {
	if state.options.StatusChanged != nil {
		state.options.StatusChanged(state.Status())
	}
}
