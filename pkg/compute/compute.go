// Package compute creates App-owned compute resources. Dispatch through App.OnCompute
// so GPU writes precede ordinary scene rendering on native and browser targets.
package compute

import (
	"github.com/karty-game/karty-webgpu/internal/computedata"
	"github.com/karty-game/karty-webgpu/pkg/shader"
	"github.com/karty-game/karty-webgpu/pkg/texture"
)

// PipelineDesc describes a program, entry point and explicit group-zero layout.
type PipelineDesc = computedata.PipelineDesc

// Pipeline is a reusable App-owned compute pipeline.
type Pipeline = computedata.Pipeline

// BindGroup binds concrete resources to one pipeline's layout.
type BindGroup = computedata.BindGroup

// Owner is implemented by engine.App. All resources are declared before Run.
type Owner interface {
	NewComputePipeline(desc PipelineDesc) (*Pipeline, error)
	NewStorageTexture(width, height int) (*texture.Texture, error)
	NewBuffer(desc shader.BufferDesc) (*shader.Buffer, error)
}

// NewPipeline declares a pipeline; device-specific compilation happens during Run setup.
func NewPipeline(owner Owner, desc PipelineDesc) (*Pipeline, error) {
	return owner.NewComputePipeline(desc)
}

// NewTexture creates a one-mip linear rgba8unorm storage texture. It can be bound
// as a material's AlbedoTexture and written by a compute pipeline before rendering.
func NewTexture(owner Owner, width, height int) (*texture.Texture, error) {
	return owner.NewStorageTexture(width, height)
}

// NewStorageBytes creates an initialized read/write GPU storage buffer from explicit bytes.
func NewStorageBytes(owner Owner, data []byte) (*shader.Buffer, error) {
	return owner.NewBuffer(shader.BufferDesc{Label: "storage", Usage: shader.StorageBuffer, Data: data})
}
