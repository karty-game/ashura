//go:build linux

package native

import (
	"fmt"

	"github.com/gogpu/gputypes"
	"github.com/karty-game/ashura/internal/platform"
	"github.com/karty-game/ashura/internal/render"
	"github.com/karty-game/ashura/internal/texturedata"
	wgpu "github.com/karty-game/ashura/internal/webgpu"
)

type gpuDecals struct {
	uploads    render.DecalUploads
	authored   gpuStaticDecals
	shader     func(string, bool) string
	uniform    *wgpu.Buffer
	atlas      *texturedata.Texture
	groups     [4]*wgpu.BindGroup
	groupIndex uint32
}

func RegisterDecals(host any) error {
	driver, err := materialFeatureDriver(host)
	if err != nil {
		return err
	}

	if driver.decals != nil {
		return fmt.Errorf("%s: %w", platform.DecalFeature, platform.ErrFeatureDuplicate)
	}

	buffer, err := driver.device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "decal projectors", Size: render.DecalUniformSize,
		Usage: gputypes.BufferUsageUniform | gputypes.BufferUsageCopyDst,
	})
	if err != nil {
		return fmt.Errorf("create decal uniform: %w", err)
	}

	group := uint32(2)
	if driver.clusteredLights != nil {
		group++
	}

	driver.decals = &gpuDecals{uniform: buffer, groupIndex: group, shader: render.DecalMaterialShader}

	return driver.decals.authored.prepare(driver.device)
}

func (driver *Driver) prepareDecals(frame *render.DecalFrame) error {
	decals := driver.decals
	if decals == nil {
		if frame != nil {
			return platform.ErrFeatureMissing
		}

		return nil
	}

	if err := decals.uploads.Update(frame, func(offset int, data []byte) error {
		return driver.queue.WriteBuffer(
			decals.uniform,
			uint64(offset), //nolint:gosec // Offsets come from bounded GPU record indices.
			data,
		)
	}); err != nil {
		return fmt.Errorf("upload decal projectors: %w", err)
	}

	if err := driver.initMaterials(); err != nil {
		return err
	}

	atlas := decals.atlas
	if atlas == nil {
		atlas = driver.white
	}

	if frame != nil {
		atlas = frame.Atlas
	}

	if atlas == decals.atlas {
		return nil
	}

	return driver.bindDecalAtlas(atlas)
}

//nolint:mnd // Binding numbers identify the decal render shader ABI.
func (driver *Driver) bindDecalAtlas(atlas *texturedata.Texture) error {
	decals := driver.decals

	view, err := driver.textureView(atlas)
	if err != nil {
		return fmt.Errorf("prepare decal atlas: %w", err)
	}

	var groups [4]*wgpu.BindGroup
	for variant := range groups {
		pipeline := driver.materialPipeline(variant >= 2, variant%2 == 1)
		if pipeline == nil {
			continue
		}

		layout := pipeline.GetBindGroupLayout(decals.groupIndex)
		groups[variant], err = driver.device.CreateBindGroup(&wgpu.BindGroupDescriptor{
			Layout: layout,
			Entries: []wgpu.BindGroupEntry{
				{Binding: 0, Buffer: decals.uniform, Size: render.DecalUniformSize},
				{Binding: 1, TextureView: view},
				{Binding: 2, Buffer: decals.authored.view, Size: render.DecalViewBytes},
				{Binding: 3, Buffer: decals.authored.records, Size: render.StaticDecalBytes},
				{Binding: 4, Buffer: decals.authored.tiles, Size: render.DecalTileBytes},
				{Binding: 5, Buffer: decals.authored.masks, Size: render.DecalTileStorageBytes},
			},
		})
		layout.Release()

		if err != nil {
			releaseDecalGroups(groups)

			return fmt.Errorf("bind decal atlas: %w", err)
		}
	}

	releaseDecalGroups(decals.groups)
	decals.atlas, decals.groups = atlas, groups

	return nil
}

func (driver *Driver) bindDecals(pass *wgpu.RenderPassEncoder, draw render.DrawCall) {
	if driver.decals == nil || draw.Impostor != nil {
		return
	}

	pass.SetBindGroup(driver.decals.groupIndex, driver.decals.groups[draw.DecalVariant()], nil)
}

func releaseDecalGroups(groups [4]*wgpu.BindGroup) {
	for _, group := range groups {
		if group != nil {
			group.Release()
		}
	}
}

func (driver *Driver) clearDecalAtlas(atlas *texturedata.Texture) {
	if driver.decals != nil && driver.decals.atlas == atlas {
		releaseDecalGroups(driver.decals.groups)
		driver.decals.groups = [4]*wgpu.BindGroup{}
		driver.decals.atlas = nil
	}
}

func (driver *Driver) closeDecals() {
	if driver.decals != nil {
		releaseDecalGroups(driver.decals.groups)
		driver.decals.authored.release()
		driver.decals.uniform.Release()
		driver.decals = nil
	}
}
