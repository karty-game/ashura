// Package computedata defines backend-independent compute descriptors and handles.
package computedata

import (
	"errors"

	"github.com/karty-game/karty-webgpu/internal/bufferdata"
	"github.com/karty-game/karty-webgpu/pkg/shader"
)

const MaxWorkgroups uint32 = 65535

// PipelineDesc declares one immutable WGSL compute pipeline and group-zero layout.
type PipelineDesc struct {
	Program    *shader.Module
	EntryPoint string
	Layout     shader.Layout
}

// GroupOwner creates and tracks App-owned resource bindings.
type GroupOwner interface {
	NewComputeBindings(pipeline *Pipeline, bindings []shader.Binding) (*BindGroup, error)
}

// Pipeline is an App-owned compute pipeline descriptor with reusable device state.
type Pipeline struct {
	owner    GroupOwner
	desc     PipelineDesc
	released bool
}

// BindGroup is a reusable App-owned binding set. Unload it before its resources.
type BindGroup struct {
	pipeline *Pipeline
	bindings []shader.Binding
	released bool
}

// Resources is the setup-time device resource registry.
type Resources struct {
	Buffers    []*bufferdata.Buffer
	Pipelines  []*Pipeline
	BindGroups []*BindGroup
}

// Dispatch records group-zero work scheduled before scene rendering.
type Dispatch struct {
	Pipeline *Pipeline
	Group    *BindGroup
	X, Y, Z  uint32
}

var errPipeline = errors.New("compute: invalid program, entry point or portable binding layout")

// NewPipeline validates portable layout bounds without compiling WGSL on the CPU.
func NewPipeline(owner GroupOwner, desc PipelineDesc) (*Pipeline, error) {
	if desc.Program == nil || desc.Program.Source() == "" || desc.EntryPoint == "" {
		return nil, errPipeline
	}

	var uniform, storage, images int

	seen := make(map[uint32]bool, len(desc.Layout))
	for _, binding := range desc.Layout {
		const maxBindings = 1000
		if binding.Binding >= maxBindings || seen[binding.Binding] {
			return nil, errPipeline
		}

		seen[binding.Binding] = true
		if binding.Type != shader.StorageTexture && (binding.MinSize == 0 || binding.MinSize%bufferdata.Alignment != 0) {
			return nil, errPipeline
		}

		switch binding.Type {
		case shader.Uniform:
			uniform++

			if binding.MinSize > bufferdata.MaxUniformSize {
				return nil, errPipeline
			}
		case shader.ReadOnlyStorage, shader.ReadWriteStorage:
			storage++

			if binding.MinSize > bufferdata.MaxStorageSize {
				return nil, errPipeline
			}
		case shader.StorageTexture:
			images++

			if binding.MinSize != 0 {
				return nil, errPipeline
			}
		default:
			return nil, errPipeline
		}
	}

	const maxUniforms, maxStorage, maxImages = 12, 8, 4
	if uniform > maxUniforms || storage > maxStorage || images > maxImages {
		return nil, errPipeline
	}

	desc.Layout = append(shader.Layout(nil), desc.Layout...)

	return &Pipeline{owner: owner, desc: desc}, nil
}

// BindGroup creates reusable concrete bindings for this pipeline's group zero.
func (pipeline *Pipeline) BindGroup(bindings ...shader.Binding) (*BindGroup, error) {
	if !ValidPipeline(pipeline) {
		return nil, errPipeline
	}

	return pipeline.owner.NewComputeBindings(pipeline, bindings)
}

// NewBindGroup copies already admitted resource references.
func NewBindGroup(pipeline *Pipeline, bindings []shader.Binding) *BindGroup {
	return &BindGroup{pipeline: pipeline, bindings: append([]shader.Binding(nil), bindings...)}
}

// Description exposes immutable setup data to drivers.
func Description(pipeline *Pipeline) PipelineDesc { return pipeline.desc }

// Bindings exposes immutable binding references to drivers.
func Bindings(group *BindGroup) []shader.Binding { return group.bindings }

// PipelineFor returns the pipeline a group was created against.
func PipelineFor(group *BindGroup) *Pipeline { return group.pipeline }

// ValidPipeline reports whether a pipeline handle is live.
func ValidPipeline(pipeline *Pipeline) bool { return pipeline != nil && !pipeline.released }

// ValidGroup reports whether a group handle is live.
func ValidGroup(group *BindGroup) bool { return group != nil && !group.released }

// ReleasePipeline invalidates a pipeline after driver resources are released.
func ReleasePipeline(pipeline *Pipeline) { pipeline.released = true; pipeline.owner = nil }

// ReleaseGroup invalidates a bind group after driver resources are released.
func ReleaseGroup(group *BindGroup) { group.released = true; group.bindings = nil }
