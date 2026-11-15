//go:build linux

package native

import (
	"fmt"

	"github.com/gogpu/gputypes"
	"github.com/karty-game/karty-webgpu/internal/render"
	wgpu "github.com/karty-game/karty-webgpu/internal/webgpu"
)

const (
	pointShadowVertexEntry    = "vs_main"
	pointShadowTextureBinding = 2
)

// gpuPointShadow stores only the material sampling ABI shared by optional renderers.
type gpuPointShadow struct {
	resolution    uint32
	image         *wgpu.Texture
	cube          *wgpu.TextureView
	sampleUniform *wgpu.Buffer
	sampler       *wgpu.Sampler
	sampleGroups  [4]*wgpu.BindGroup
	sampleData    [render.PointShadowSampleSize]byte
}

// preparePointShadowSampling retains the common material binding ABI only.
// Optional shadow passes and their WGSL are installed by RegisterPointShadows.
func (driver *Driver) preparePointShadowSampling(resolution uint32) error {
	if driver.standardPipeline == nil && driver.sourcePipeline == nil {
		return nil
	}

	if resolution == 0 {
		resolution = 1
	}

	return driver.initPointShadow(resolution)
}

func (driver *Driver) initPointShadow(resolution uint32) error {
	if driver.pointShadow != nil && driver.pointShadow.resolution == resolution {
		return nil
	}

	if driver.pointShadow != nil {
		driver.pointShadow.release()
	}

	shadow := &gpuPointShadow{resolution: resolution}
	driver.pointShadow = shadow

	var err error

	shadow.image, err = driver.device.CreateTexture(
		&wgpu.TextureDescriptor{
			Label:         "point shadow six faces",
			Usage:         gputypes.TextureUsageRenderAttachment | gputypes.TextureUsageTextureBinding,
			Dimension:     gputypes.TextureDimension2D,
			Size:          gputypes.Extent3D{Width: resolution, Height: resolution, DepthOrArrayLayers: render.PointShadowFaces},
			Format:        gputypes.TextureFormatDepth32Float,
			MipLevelCount: 1,
			SampleCount:   1,
		},
	)
	if err != nil {
		return fmt.Errorf("create point shadow: %w", err)
	}

	shadow.cube, err = shadow.image.CreateView(
		&wgpu.TextureViewDescriptor{
			Dimension:       gputypes.TextureViewDimensionCube,
			MipLevelCount:   1,
			ArrayLayerCount: render.PointShadowFaces,
		},
	)
	if err != nil {
		return err
	}

	if err := driver.initPointShadowSampleBindings(shadow); err != nil {
		return err
	}

	return nil
}

func (driver *Driver) initPointShadowSampleBindings(shadow *gpuPointShadow) error {
	var err error

	shadow.sampler, err = driver.device.CreateSampler(
		&wgpu.SamplerDescriptor{
			Compare:      gputypes.CompareFunctionLessEqual,
			AddressModeU: gputypes.AddressModeClampToEdge,
			AddressModeV: gputypes.AddressModeClampToEdge,
			AddressModeW: gputypes.AddressModeClampToEdge,
			MinFilter:    gputypes.FilterModeLinear,
			MagFilter:    gputypes.FilterModeLinear,
			MipmapFilter: gputypes.MipmapFilterModeNearest,
		},
	)
	if err != nil {
		return err
	}

	shadow.sampleUniform, err = driver.device.CreateBuffer(
		&wgpu.BufferDescriptor{Usage: gputypes.BufferUsageUniform | gputypes.BufferUsageCopyDst, Size: render.PointShadowSampleSize},
	)
	if err != nil {
		return err
	}

	for variant := range 4 {
		pipeline := driver.materialPipeline(variant >= 2, variant%2 == 1)
		if pipeline == nil {
			continue
		}

		layout := pipeline.GetBindGroupLayout(1)
		shadow.sampleGroups[variant], err = driver.device.CreateBindGroup(
			&wgpu.BindGroupDescriptor{
				Layout: layout,
				Entries: []wgpu.BindGroupEntry{
					{Binding: 0, Buffer: shadow.sampleUniform, Size: render.PointShadowSampleSize},
					{Binding: 1, Sampler: shadow.sampler},
					{Binding: pointShadowTextureBinding, TextureView: shadow.cube},
				},
			},
		)
		layout.Release()

		if err != nil {
			return err
		}
	}

	return nil
}

func (driver *Driver) bindPointShadow(pass *wgpu.RenderPassEncoder, draw render.DrawCall) {
	if draw.Impostor != nil {
		return
	}

	variant := 0
	if draw.CullBackfaces {
		variant = 1
	}

	if draw.Source {
		variant += 2
	}

	pass.SetBindGroup(1, driver.pointShadow.sampleGroups[variant], nil)
}

func (shadow *gpuPointShadow) release() {
	for _, group := range shadow.sampleGroups {
		if group != nil {
			group.Release()
		}
	}

	if shadow.sampleUniform != nil {
		shadow.sampleUniform.Release()
	}

	if shadow.sampler != nil {
		shadow.sampler.Release()
	}

	if shadow.cube != nil {
		shadow.cube.Release()
	}

	if shadow.image != nil {
		shadow.image.Release()
	}
}
