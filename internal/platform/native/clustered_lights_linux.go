//go:build linux

package native

import (
	"fmt"

	"github.com/gogpu/gputypes"
	"github.com/karty-game/ashura/internal/render"
	wgpu "github.com/karty-game/ashura/internal/webgpu"
)

type gpuClusteredLights struct {
	state          render.ClusterFrameState
	spots          render.SpotShadowState
	uniform, masks *wgpu.Buffer
	build          *wgpu.ComputePipeline
	buildGroup     *wgpu.BindGroup
	materials      [4]*wgpu.BindGroup
	spotPipeline   *wgpu.RenderPipeline
	spotTargets    [render.SpotShadowCount]gpuSpotTarget
}

type gpuSpotTarget struct {
	image   *wgpu.Texture
	view    *wgpu.TextureView
	uniform *wgpu.Buffer
	group   *wgpu.BindGroup
	matrix  [render.SpotMatrixBytes]byte
}

func RegisterClusteredLights(host any) error {
	driver, err := materialFeatureDriver(host)
	if err != nil {
		return err
	}

	lighting := &gpuClusteredLights{}

	driver.clusteredLights = lighting
	if err := lighting.prepareBuild(driver.device); err != nil {
		return err
	}

	return lighting.prepareSpots(driver.device)
}

func (lighting *gpuClusteredLights) prepareBuild(device *wgpu.Device) error {
	var err error

	lighting.uniform, err = device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "clustered light uniforms", Size: render.ClusterUniformSize,
		Usage: gputypes.BufferUsageUniform | gputypes.BufferUsageCopyDst,
	})
	if err != nil {
		return err
	}

	lighting.masks, err = device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "clustered light masks", Size: render.ClusterMaskBytes, Usage: gputypes.BufferUsageStorage,
	})
	if err != nil {
		return err
	}

	module, err := device.CreateShaderModuleWGSL(render.ClusterBuildShader())
	if err != nil {
		return err
	}
	defer module.Release()

	lighting.build, err = device.CreateComputePipeline(&wgpu.ComputePipelineDescriptor{
		Label: "clustered light culling", Module: module, EntryPoint: "buildLightMasks",
	})
	if err != nil {
		return err
	}

	layout := lighting.build.GetBindGroupLayout(0)
	defer layout.Release()

	lighting.buildGroup, err = device.CreateBindGroup(&wgpu.BindGroupDescriptor{
		Layout: layout, Entries: lighting.bufferEntries(),
	})

	return err
}

func (lighting *gpuClusteredLights) bufferEntries() []wgpu.BindGroupEntry {
	return []wgpu.BindGroupEntry{
		{Binding: 0, Buffer: lighting.uniform, Size: render.ClusterUniformSize},
		{Binding: 1, Buffer: lighting.masks, Size: render.ClusterMaskBytes},
	}
}

func (driver *Driver) prepareClusterMaterialGroups() error {
	lighting := driver.clusteredLights
	if lighting == nil {
		return nil
	}

	const firstSpotBinding, secondSpotBinding = 2, 3

	for variant := range lighting.materials {
		pipeline := driver.materialPipeline(variant >= 2, variant%2 == 1)
		if pipeline == nil {
			continue
		}

		layout := pipeline.GetBindGroupLayout(2)
		group, err := driver.device.CreateBindGroup(&wgpu.BindGroupDescriptor{
			Layout: layout, Entries: append(lighting.bufferEntries(),
				wgpu.BindGroupEntry{Binding: firstSpotBinding, TextureView: lighting.spotTargets[0].view},
				wgpu.BindGroupEntry{Binding: secondSpotBinding, TextureView: lighting.spotTargets[1].view}),
		})
		layout.Release()

		if err != nil {
			return err
		}

		lighting.materials[variant] = group
	}

	return nil
}

func (driver *Driver) recordLighting(encoder *wgpu.CommandEncoder, frame render.Frame) error {
	// Admit the draw batch once for the depth and color passes.
	if err := driver.geometry.Prepare(frame.Draws); err != nil {
		return err
	}

	if err := driver.recordPointShadow(encoder, frame); err != nil {
		return err
	}

	if err := driver.recordStaticDecals(encoder, frame); err != nil {
		return err
	}

	return driver.recordClusteredLighting(encoder, frame)
}

func (driver *Driver) recordClusteredLighting(encoder *wgpu.CommandEncoder, frame render.Frame) error {
	lighting := driver.clusteredLights
	if lighting == nil || len(frame.Draws) == 0 {
		return nil
	}

	dirty, masks := lighting.spots.Update(frame)

	upload, dispatch := lighting.state.Update(frame, driver.width, driver.height, lighting.spots.Views)
	if upload {
		if err := driver.queue.WriteBuffer(lighting.uniform, 0, lighting.state.Data[:]); err != nil {
			lighting.state.Invalidate()

			return err
		}
	}

	if dispatch {
		if err := lighting.buildMasks(encoder); err != nil {
			lighting.state.Invalidate()

			return err
		}
	}

	if dispatch {
		driver.metrics.Dispatch()
	}

	if dirty != 0 {
		if err := driver.drawSpotShadows(encoder, frame, dirty, masks); err != nil {
			return fmt.Errorf("spot shadows: %w", err)
		}
	}

	return nil
}

func (lighting *gpuClusteredLights) buildMasks(encoder *wgpu.CommandEncoder) error {
	pass, err := encoder.BeginComputePass(&wgpu.ComputePassDescriptor{Label: "clustered light culling"})
	if err != nil {
		return err
	}
	defer pass.Release()

	pass.SetPipeline(lighting.build)
	pass.SetBindGroup(0, lighting.buildGroup, nil)
	groups := lighting.state.Groups
	pass.DispatchWorkgroups(groups[0], groups[1], groups[2])
	pass.End()

	return nil
}

func (lighting *gpuClusteredLights) release() {
	for _, group := range lighting.materials {
		if group != nil {
			group.Release()
		}
	}

	for index := range lighting.spotTargets {
		lighting.spotTargets[index].release()
	}

	if lighting.spotPipeline != nil {
		lighting.spotPipeline.Release()
	}

	if lighting.buildGroup != nil {
		lighting.buildGroup.Release()
	}

	if lighting.build != nil {
		lighting.build.Release()
	}

	if lighting.uniform != nil {
		lighting.uniform.Release()
	}

	if lighting.masks != nil {
		lighting.masks.Release()
	}
}
