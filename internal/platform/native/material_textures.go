//go:build linux

package native

import (
	"github.com/gogpu/gputypes"
	"github.com/karty-game/karty-webgpu/internal/render"
	"github.com/karty-game/karty-webgpu/internal/texturedata"
	wgpu "github.com/karty-game/karty-webgpu/internal/webgpu"
)

func (driver *Driver) materialTextures(draw render.DrawCall) (albedo, normal, occlusion, reflection *texturedata.Texture) {
	albedo, normal, occlusion, reflection = draw.Albedo, draw.Normal, draw.AO, draw.Reflection

	if albedo == nil {
		albedo = driver.white
	}

	if normal == nil {
		normal = driver.flat
	}

	if draw.Source || draw.Impostor != nil {
		occlusion = nil
	} else if occlusion == nil {
		occlusion = driver.whiteAO
	}

	if reflection == nil {
		reflection = driver.white
	}

	return albedo, normal, occlusion, reflection
}

func (driver *Driver) standardTextureBindings(
	entries []wgpu.BindGroupEntry,
	occlusion, reflection, lightmap *texturedata.Texture,
) ([]wgpu.BindGroupEntry, error) {
	aoView, err := driver.textureView(occlusion)
	if err != nil {
		return nil, err
	}

	reflectionView, err := driver.textureView(reflection)
	if err != nil {
		return nil, err
	}

	lightmapView, err := driver.textureView(lightmap)
	if err != nil {
		return nil, err
	}

	if err := driver.initLightmapSampler(); err != nil {
		return nil, err
	}

	return append(entries,
		wgpu.BindGroupEntry{Binding: render.OcclusionBinding, TextureView: aoView},
		wgpu.BindGroupEntry{Binding: render.ReflectionBinding, TextureView: reflectionView},
		wgpu.BindGroupEntry{Binding: render.LightmapBinding, TextureView: lightmapView},
		wgpu.BindGroupEntry{Binding: render.LightmapSamplerBinding, Sampler: driver.lightmapSampler},
	), nil
}

func (driver *Driver) initLightmapSampler() error {
	if driver.lightmapSampler == nil {
		var err error

		driver.lightmapSampler, err = driver.device.CreateSampler(
			&wgpu.SamplerDescriptor{
				AddressModeU: gputypes.AddressModeClampToEdge,
				AddressModeV: gputypes.AddressModeClampToEdge,
				AddressModeW: gputypes.AddressModeClampToEdge,
				MagFilter:    gputypes.FilterModeLinear,
				MinFilter:    gputypes.FilterModeLinear,
				MipmapFilter: gputypes.MipmapFilterModeNearest,
			},
		)
		if err != nil {
			return err
		}
	}

	return nil
}

func (driver *Driver) rnmTextureBindings(entries []wgpu.BindGroupEntry, rnm [3]*texturedata.Texture) ([]wgpu.BindGroupEntry, error) {
	if err := driver.initLightmapSampler(); err != nil {
		return nil, err
	}

	for index, image := range rnm {
		if image == nil {
			image = driver.whiteAO
		}

		view, err := driver.textureView(image)
		if err != nil {
			return nil, err
		}

		entries = append(entries, wgpu.BindGroupEntry{Binding: uint32(render.RNMLightmapBinding + index), TextureView: view})
	}

	return append(entries, wgpu.BindGroupEntry{Binding: render.LightmapSamplerBinding, Sampler: driver.lightmapSampler}), nil
}
