package engine

import (
	"github.com/karty-game/ashura/internal/bufferdata"
	"github.com/karty-game/ashura/internal/computedata"
	"github.com/karty-game/ashura/pkg/compute"
	"github.com/karty-game/ashura/pkg/shader"
	"github.com/karty-game/ashura/pkg/texture"
)

// UnloadComputeBindings releases a binding set. Queued dispatches must finish the
// current callback phase first; submitted GPU work retains its backend references.
func (app *App) UnloadComputeBindings(group *compute.BindGroup) error {
	if app == nil || app.closed {
		return errAppClosed
	}

	if _, owned := app.computeGroups[group]; !owned || !computedata.ValidGroup(group) {
		return errComputeOwner
	}

	if app.computeActive {
		for _, command := range app.computeFrame.commands {
			if command.Group == group {
				return errComputeInUse
			}
		}
	}

	if app.driver != nil {
		app.driver.UnloadComputeBindings(group)
	}

	computedata.ReleaseGroup(group)
	delete(app.computeGroups, group)

	return nil
}

// UnloadComputePipeline releases a pipeline after its binding sets have been unloaded.
func (app *App) UnloadComputePipeline(pipeline *compute.Pipeline) error {
	if app == nil || app.closed {
		return errAppClosed
	}

	if _, owned := app.computePipelines[pipeline]; !owned || !computedata.ValidPipeline(pipeline) {
		return errComputeOwner
	}

	for group := range app.computeGroups {
		if computedata.PipelineFor(group) == pipeline {
			return errComputeInUse
		}
	}

	if app.driver != nil {
		app.driver.UnloadComputePipeline(pipeline)
	}

	computedata.ReleasePipeline(pipeline)
	delete(app.computePipelines, pipeline)

	return nil
}

// UnloadBuffer releases a buffer after its binding sets have been unloaded.
func (app *App) UnloadBuffer(buffer *shader.Buffer) error {
	if app == nil || app.closed {
		return errAppClosed
	}

	if _, owned := app.buffers[buffer]; !owned || !bufferdata.Valid(buffer) {
		return errComputeOwner
	}

	for group := range app.computeGroups {
		for _, binding := range computedata.Bindings(group) {
			if binding.Buffer == buffer {
				return errComputeInUse
			}
		}
	}

	if app.driver != nil {
		app.driver.UnloadBuffer(buffer)
	}

	bufferdata.Release(buffer)
	delete(app.buffers, buffer)

	return nil
}

func (app *App) computeTextureInUse(image *texture.Texture) bool {
	for group := range app.computeGroups {
		for _, binding := range computedata.Bindings(group) {
			if binding.Texture == image {
				return true
			}
		}
	}

	return false
}

func (app *App) closeCompute() {
	for group := range app.computeGroups {
		computedata.ReleaseGroup(group)
	}

	for pipeline := range app.computePipelines {
		computedata.ReleasePipeline(pipeline)
	}

	for buffer := range app.buffers {
		bufferdata.Release(buffer)
	}

	app.computeUpdates.clear()
	app.computeGroups = nil
	app.computePipelines = nil
	app.buffers = nil
	app.dirtyBuffers = nil
	app.bufferUploads = nil
	app.computeFrame.commands = nil
	app.computeActive = false
}
