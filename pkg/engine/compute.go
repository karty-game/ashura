package engine

import (
	"errors"
	"fmt"

	"github.com/karty-game/karty-webgpu/internal/bufferdata"
	"github.com/karty-game/karty-webgpu/internal/computedata"
	"github.com/karty-game/karty-webgpu/internal/texturedata"
	"github.com/karty-game/karty-webgpu/pkg/compute"
	"github.com/karty-game/karty-webgpu/pkg/shader"
	"github.com/karty-game/karty-webgpu/pkg/texture"
)

var (
	errGPUSetup        = fmt.Errorf("engine: declare compute resources before Run: %w", ErrPhase)
	errComputeOwner    = ErrOwner
	errComputeBindings = errors.New("engine: compute bindings do not match the declared layout")
	errComputeInUse    = fmt.Errorf("engine: compute resource is still bound or queued: %w", ErrInUse)
	errComputePhase    = fmt.Errorf("engine: dispatch is only valid inside OnCompute: %w", ErrPhase)
	errComputeDispatch = errors.New("engine: dispatch exceeds the portable workgroup limit")
)

type bufferOwner struct{ app *App }

func (owner bufferOwner) MarkBufferDirty(buffer *bufferdata.Buffer) {
	owner.app.dirtyBuffers = append(owner.app.dirtyBuffers, buffer)
}

// NewBuffer declares an App-owned GPU buffer before Run. Prefer the shader or
// compute constructors. CPU staging has Size bytes; writes upload only dirty ranges.
func (app *App) NewBuffer(desc shader.BufferDesc) (*shader.Buffer, error) {
	if err := app.checkGPUSetup(); err != nil {
		return nil, err
	}

	buffer, err := bufferdata.Create(bufferOwner{app}, desc)
	if err != nil {
		return nil, err
	}

	if app.buffers == nil {
		app.buffers = make(map[*shader.Buffer]struct{})
	}

	app.buffers[buffer] = struct{}{}

	return buffer, nil
}

// NewStorageTexture declares a single-mip linear GPU output before Run. Use
// compute.NewTexture for the discoverable compute-package constructor.
func (app *App) NewStorageTexture(width, height int) (*texture.Texture, error) {
	if err := app.checkGPUSetup(); err != nil {
		return nil, err
	}

	image, err := texturedata.CreateStorage(width, height)
	if err != nil {
		return nil, err
	}

	if app.textures == nil {
		app.textures = make(map[*texture.Texture]struct{})
	}

	app.textures[image] = struct{}{}

	return image, nil
}

// NewComputePipeline declares a pipeline; device compilation happens once at Run setup.
func (app *App) NewComputePipeline(desc compute.PipelineDesc) (*compute.Pipeline, error) {
	if err := app.checkGPUSetup(); err != nil {
		return nil, err
	}

	pipeline, err := computedata.NewPipeline(app, desc)
	if err != nil {
		return nil, err
	}

	if app.computePipelines == nil {
		app.computePipelines = make(map[*compute.Pipeline]struct{})
	}

	app.computePipelines[pipeline] = struct{}{}

	return pipeline, nil
}

// NewComputeBindings creates owned group-zero bindings. Prefer pipeline.BindGroup.
func (app *App) NewComputeBindings(pipeline *compute.Pipeline, bindings []shader.Binding) (*compute.BindGroup, error) {
	if err := app.checkGPUSetup(); err != nil {
		return nil, err
	}

	if _, owned := app.computePipelines[pipeline]; !owned || !computedata.ValidPipeline(pipeline) {
		return nil, errComputeOwner
	}

	if err := app.admitComputeBindings(pipeline, bindings); err != nil {
		return nil, err
	}

	group := computedata.NewBindGroup(pipeline, bindings)

	if app.computeGroups == nil {
		app.computeGroups = make(map[*compute.BindGroup]struct{})
	}

	app.computeGroups[group] = struct{}{}

	return group, nil
}

func (app *App) checkGPUSetup() error {
	if app == nil || app.closed {
		return errAppClosed
	}

	if app.driver != nil {
		return errGPUSetup
	}

	return nil
}

func (app *App) admitComputeBindings(pipeline *compute.Pipeline, bindings []shader.Binding) error {
	layout := computedata.Description(pipeline).Layout
	if len(layout) != len(bindings) {
		return errComputeBindings
	}

	supplied := make(map[uint32]shader.Binding, len(bindings))
	for _, binding := range bindings {
		if _, duplicate := supplied[binding.Binding]; duplicate {
			return errComputeBindings
		}

		supplied[binding.Binding] = binding
	}

	bufferUses := make(map[*shader.Buffer]shader.BindingType)
	textureUses := make(map[*texture.Texture]bool)

	for _, slot := range layout {
		binding, found := supplied[slot.Binding]
		if !found {
			return errComputeBindings
		}

		if slot.Type == shader.StorageTexture {
			if err := app.admitStorageTexture(binding, textureUses); err != nil {
				return err
			}
		} else if err := app.admitBufferBinding(slot, binding, bufferUses); err != nil {
			return err
		}
	}

	return nil
}

func (app *App) admitStorageTexture(binding shader.Binding, used map[*texture.Texture]bool) error {
	if binding.Buffer != nil || binding.Texture == nil || used[binding.Texture] {
		return errComputeBindings
	}

	if err := app.validateTextures(binding.Texture); err != nil {
		return err
	}

	if !texturedata.IsStorage(binding.Texture) {
		return errComputeBindings
	}

	used[binding.Texture] = true

	return nil
}

func (app *App) admitBufferBinding(slot shader.BindingLayout, binding shader.Binding, used map[*shader.Buffer]shader.BindingType) error {
	if binding.Texture != nil || binding.Buffer == nil {
		return errComputeBindings
	}

	if _, owned := app.buffers[binding.Buffer]; !owned || !bufferdata.Valid(binding.Buffer) {
		return errComputeOwner
	}

	wantUsage := bufferdata.Storage
	if slot.Type == shader.Uniform {
		wantUsage = bufferdata.Uniform
	}

	if bufferdata.Kind(binding.Buffer) != wantUsage || binding.Buffer.Size() < slot.MinSize {
		return errComputeBindings
	}

	if previous, duplicate := used[binding.Buffer]; duplicate &&
		(previous == shader.ReadWriteStorage || slot.Type == shader.ReadWriteStorage) {
		return errComputeBindings
	}

	used[binding.Buffer] = slot.Type

	return nil
}

// OnCompute registers GPU work recorded after CPU updates and before scene rendering.
// Pipelines/resources are declared before Run. Buffer writes in either callback
// phase upload before all dispatches in that frame. Returning an error stops Run.
// The returned idempotent cancellation function prevents future calls and releases
// the callback. Registrations during compute start on the next compute phase;
// cancellation takes effect immediately. Like OnUpdate, registration and
// cancellation belong to the App loop goroutine; nil/closed registrations are inert.
func (app *App) OnCompute(work func(*Frame) error) func() {
	if app == nil || app.closed || work == nil {
		return func() {}
	}

	return app.computeUpdates.add(work)
}

// Frame is a borrowed compute recorder, valid only during an OnCompute callback.
// It never exposes a backend encoder or performs a CPU/GPU synchronization wait.
type Frame struct {
	owner    *App
	commands []computedata.Dispatch
}

// Dispatch schedules group-zero work before rendering. Zero workgroups are a no-op.
func (frame *Frame) Dispatch(pipeline *compute.Pipeline, group *compute.BindGroup, groupsX, groupsY, groupsZ uint32) error {
	if frame == nil || frame.owner == nil || frame.owner.closed || !frame.owner.computeActive {
		return errComputePhase
	}

	if err := frame.owner.checkComputeGroup(pipeline, group); err != nil {
		return err
	}

	if groupsX > computedata.MaxWorkgroups || groupsY > computedata.MaxWorkgroups || groupsZ > computedata.MaxWorkgroups {
		return errComputeDispatch
	}

	if groupsX == 0 || groupsY == 0 || groupsZ == 0 {
		return nil
	}

	owner := frame.owner
	owner.computeFrame.commands = append(
		owner.computeFrame.commands,
		computedata.Dispatch{Pipeline: pipeline, Group: group, X: groupsX, Y: groupsY, Z: groupsZ},
	)

	return nil
}

func (app *App) checkComputeGroup(pipeline *compute.Pipeline, group *compute.BindGroup) error {
	if _, owned := app.computePipelines[pipeline]; !owned || !computedata.ValidPipeline(pipeline) {
		return errComputeOwner
	}

	if _, owned := app.computeGroups[group]; !owned || !computedata.ValidGroup(group) || computedata.PipelineFor(group) != pipeline {
		return errComputeOwner
	}

	return nil
}

func (app *App) runCompute() error {
	app.computeFrame.owner = app
	clear(app.computeFrame.commands)
	app.computeFrame.commands = app.computeFrame.commands[:0]

	app.computeActive = true
	defer func() { app.computeActive = false }()

	entries := app.computeUpdates.begin()
	defer app.computeUpdates.end()

	for _, entry := range entries {
		if !entry.active {
			continue
		}

		if err := entry.callback(&app.computeFrame); err != nil {
			return fmt.Errorf("compute callback: %w", err)
		}

		if app.closed {
			return errAppClosed
		}
	}

	return nil
}

func (app *App) collectUploads() []bufferdata.Upload {
	clear(app.bufferUploads)

	app.bufferUploads = app.bufferUploads[:0]
	for _, buffer := range app.dirtyBuffers {
		if _, owned := app.buffers[buffer]; owned && bufferdata.Valid(buffer) {
			app.bufferUploads = bufferdata.Pending(buffer, app.bufferUploads)
		}
	}

	clear(app.dirtyBuffers)
	app.dirtyBuffers = app.dirtyBuffers[:0]

	return app.bufferUploads
}

func (app *App) computeResources() computedata.Resources {
	var resources computedata.Resources
	for buffer := range app.buffers {
		resources.Buffers = append(resources.Buffers, buffer)
	}

	for pipeline := range app.computePipelines {
		resources.Pipelines = append(resources.Pipelines, pipeline)
	}

	for group := range app.computeGroups {
		resources.BindGroups = append(resources.BindGroups, group)
	}

	return resources
}
