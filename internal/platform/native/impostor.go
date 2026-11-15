//go:build linux

package native

import (
	"fmt"

	"github.com/gogpu/gputypes"
	"github.com/karty-game/karty-webgpu/internal/impostordata"
	"github.com/karty-game/karty-webgpu/internal/render"
	wgpu "github.com/karty-game/karty-webgpu/internal/webgpu"
)

type gpuImpostor struct {
	buffer     *wgpu.Buffer
	references int
	owner      *Driver
	config     *impostordata.Document
}

func createImpostorPipeline(driver *Driver) error {
	shader, err := driver.device.CreateShaderModuleWGSL(render.ImpostorShader())
	if err != nil {
		return fmt.Errorf("create impostor.wgsl: %w", err)
	}
	defer shader.Release()

	return createPipelineVariant(driver, "impostor.wgsl", false, false, true, shader)
}

func (driver *Driver) drawPipeline(draw render.DrawCall) *wgpu.RenderPipeline {
	if draw.Impostor != nil {
		return driver.impostorPipeline
	}

	return driver.materialPipeline(draw.Source, draw.CullBackfaces)
}

// acquireImpostor uploads the already packed immutable view table once for its
// resident configuration. Further matching draw batches share the same buffer.
func (driver *Driver) acquireImpostor(config *impostordata.Document) (*gpuImpostor, error) {
	if cached := driver.impostorViews[config]; cached != nil {
		cached.references++

		return cached, nil
	}

	if driver.impostorViews == nil {
		driver.impostorViews = make(map[*impostordata.Document]*gpuImpostor)
	}

	if driver.impostorSampler == nil {
		sampler, err := driver.device.CreateSampler(&wgpu.SamplerDescriptor{
			AddressModeU: gputypes.AddressModeClampToEdge,
			AddressModeV: gputypes.AddressModeClampToEdge,
			AddressModeW: gputypes.AddressModeClampToEdge,
			MagFilter:    gputypes.FilterModeLinear,
			MinFilter:    gputypes.FilterModeLinear,
			MipmapFilter: gputypes.MipmapFilterModeLinear,
			LodMaxClamp:  maxSamplerLOD,
		})
		if err != nil {
			return nil, fmt.Errorf("create impostor sampler: %w", err)
		}

		driver.impostorSampler = sampler
	}

	buffer, err := createBuffer(driver, config.Uniform, gputypes.BufferUsageUniform)
	if err != nil {
		return nil, err
	}

	value := &gpuImpostor{buffer: buffer, references: 1, owner: driver, config: config}
	driver.impostorViews[config] = value

	return value, nil
}

func (value *gpuImpostor) release() {
	value.references--
	if value.references == 0 {
		value.buffer.Release()
		delete(value.owner.impostorViews, value.config)
	}
}

func (driver *Driver) releaseGraphicsPipelines() {
	for _, pipeline := range []*wgpu.RenderPipeline{driver.standardPipeline, driver.standardCulledPipeline, driver.sourceCulledPipeline, driver.sourcePipeline, driver.impostorPipeline} {
		if pipeline != nil {
			pipeline.Release()
		}
	}
}
