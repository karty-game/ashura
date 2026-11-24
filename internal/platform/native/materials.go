//go:build linux

package native

import (
	"fmt"

	"github.com/gogpu/gputypes"
	"github.com/karty-game/ashura/internal/impostordata"
	"github.com/karty-game/ashura/internal/render"
	"github.com/karty-game/ashura/internal/texturedata"
	wgpu "github.com/karty-game/ashura/internal/webgpu"
)

const maxSamplerLOD = 32

type gpuTexture struct {
	bytes uint64
	image *wgpu.Texture
	view  *wgpu.TextureView
}
type gpuMaterial struct {
	config                    *impostordata.Document
	impostor                  *gpuImpostor
	culled                    bool
	state                     render.UniformState
	uniform                   *wgpu.Buffer
	group                     *wgpu.BindGroup
	albedo, normal, occlusion *texturedata.Texture
	reflection                *texturedata.Texture
	lightmap                  *texturedata.Texture
	rnm                       [3]*texturedata.Texture
	seen                      bool
	source                    bool
}

func (driver *Driver) updateFrameUniform(frame render.Frame) error {
	if driver.frameUniform == nil {
		buffer, err := driver.device.CreateBuffer(&wgpu.BufferDescriptor{
			Label: "scene frame uniform", Usage: gputypes.BufferUsageUniform | gputypes.BufferUsageCopyDst, Size: render.FrameUniformSize,
		})
		if err != nil {
			return fmt.Errorf("create frame uniform: %w", err)
		}

		driver.frameUniform = buffer
	}

	if driver.frameUniformState.Update(frame) {
		if err := driver.queue.WriteBuffer(driver.frameUniform, 0, driver.frameUniformState.Data[:]); err != nil {
			driver.frameUniformState.Invalidate()

			return fmt.Errorf("write frame uniform: %w", err)
		}
	}

	return nil
}

func (driver *Driver) materialGroup(draw render.DrawCall) (*wgpu.BindGroup, error) {
	if err := driver.requireMaterialRenderer(draw); err != nil {
		return nil, err
	}

	if err := driver.initMaterials(); err != nil {
		return nil, err
	}

	albedo, normal, occlusion, reflection := driver.materialTextures(draw)

	lightmap := draw.Lightmap
	if lightmap == nil {
		lightmap = driver.whiteAO
	}

	entry := driver.materials[draw.ID]
	if entry != nil && (entry.source != draw.Source || entry.config != draw.Impostor) {
		entry.release()
		delete(driver.materials, draw.ID)

		entry = nil
	}

	if entry == nil {
		var err error

		entry, err = driver.createMaterial(draw)
		if err != nil {
			return nil, err
		}

		driver.materials[draw.ID] = entry
	}

	entry.seen = true
	if entry.culled != draw.CullBackfaces {
		if entry.group != nil {
			entry.group.Release()
			entry.group = nil
		}

		entry.culled = draw.CullBackfaces
	}

	if entry.bindingsChanged(albedo, normal, occlusion, reflection, lightmap, draw.RNM) {
		if err := driver.bindMaterial(entry, albedo, normal, occlusion, reflection, lightmap, draw.RNM); err != nil {
			return nil, err
		}
	}

	if entry.state.Update(draw) {
		if err := driver.queue.WriteBuffer(entry.uniform, 0, entry.state.Data[:]); err != nil {
			entry.state = render.UniformState{}

			return nil, fmt.Errorf("write material uniform: %w", err)
		}
	}

	return entry.group, nil
}

func (driver *Driver) createMaterial(draw render.DrawCall) (*gpuMaterial, error) {
	buffer, err := driver.device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "scene material uniform", Usage: gputypes.BufferUsageUniform | gputypes.BufferUsageCopyDst,
		Size: render.MaterialUniformSize,
	})
	if err != nil {
		return nil, fmt.Errorf("create material uniform: %w", err)
	}

	entry := &gpuMaterial{uniform: buffer, source: draw.Source, config: draw.Impostor}
	if draw.Impostor != nil {
		entry.impostor, err = driver.acquireImpostor(draw.Impostor)
		if err != nil {
			buffer.Release()

			return nil, err
		}
	}

	return entry, nil
}

func (driver *Driver) initMaterials() error {
	if driver.materials == nil {
		if driver.textures == nil {
			driver.textures = make(map[*texturedata.Texture]gpuTexture)
		}

		driver.materials = make(map[uint32]*gpuMaterial)
		driver.white = texturedata.Solid([4]byte{255, 255, 255, 255}, texturedata.SRGB)
		driver.flat = texturedata.Solid([4]byte{128, 128, 255, 255}, texturedata.Linear)
		driver.whiteAO = texturedata.Solid([4]byte{255, 255, 255, 255}, texturedata.Linear)
	}

	if driver.sampler == nil {
		sampler, err := driver.device.CreateSampler(
			&wgpu.SamplerDescriptor{
				AddressModeU: gputypes.AddressModeRepeat,
				AddressModeV: gputypes.AddressModeRepeat,
				AddressModeW: gputypes.AddressModeRepeat,
				MagFilter:    gputypes.FilterModeLinear,
				MinFilter:    gputypes.FilterModeLinear,
				MipmapFilter: gputypes.MipmapFilterModeLinear,
				LodMaxClamp:  maxSamplerLOD,
				Anisotropy:   render.MaterialAnisotropy,
			},
		)
		if err != nil {
			return fmt.Errorf("create material sampler: %w", err)
		}

		driver.sampler = sampler
	}

	return nil
}

func (driver *Driver) bindMaterial(
	entry *gpuMaterial,
	albedo, normal, occlusion, reflection, lightmap *texturedata.Texture,
	rnm [3]*texturedata.Texture,
) error {
	colorView, err := driver.textureView(albedo)
	if err != nil {
		return err
	}

	normalView, err := driver.textureView(normal)
	if err != nil {
		return err
	}

	pipeline := driver.materialPipeline(entry.source, entry.culled)
	if entry.config != nil {
		pipeline = driver.impostorPipeline
	}

	layout := pipeline.GetBindGroupLayout(0)
	defer layout.Release()

	entries := []wgpu.BindGroupEntry{
		{Binding: 0, Buffer: entry.uniform, Size: render.MaterialUniformSize},
		{Binding: 1, Sampler: driver.sampler},
		{Binding: render.AlbedoBinding, TextureView: colorView},
		{Binding: render.NormalBinding, TextureView: normalView},
		{Binding: render.FrameUniformBinding, Buffer: driver.frameUniform, Size: render.FrameUniformSize},
	}
	switch {
	case entry.impostor != nil:
		entries[1].Sampler = driver.impostorSampler
		entries = append(
			entries,
			wgpu.BindGroupEntry{Binding: render.OcclusionBinding, Buffer: entry.impostor.buffer, Size: impostordata.UniformSize},
		)
	case entry.source:
		entries, err = driver.rnmTextureBindings(entries, rnm)
		if err != nil {
			return err
		}
	default:
		entries, err = driver.standardTextureBindings(entries, occlusion, reflection, lightmap)
		if err != nil {
			return err
		}
	}

	group, err := driver.device.CreateBindGroup(&wgpu.BindGroupDescriptor{Layout: layout, Entries: entries})
	if err != nil {
		return fmt.Errorf("create material bindings: %w", err)
	}

	if entry.group != nil {
		entry.group.Release()
	}

	entry.group, entry.albedo, entry.normal, entry.occlusion, entry.reflection = group, albedo, normal, occlusion, reflection

	entry.lightmap, entry.rnm = lightmap, rnm

	return nil
}

func (driver *Driver) textureView(source *texturedata.Texture) (*wgpu.TextureView, error) {
	format, err := driver.nativeTextureFormat(source)
	if err != nil {
		return nil, err
	}

	if driver.textures == nil {
		driver.textures = make(map[*texturedata.Texture]gpuTexture)
	}

	if cached, ok := driver.textures[source]; ok {
		return cached.view, nil
	}

	width, height := source.Size()

	size := gputypes.Extent3D{
		Width:              uint32(width),  //nolint:gosec // Prepared payload dimensions are bounded to 4096.
		Height:             uint32(height), //nolint:gosec // Prepared payload dimensions are bounded to 4096.
		DepthOrArrayLayers: 1,
	}

	usage := gputypes.TextureUsageTextureBinding | gputypes.TextureUsageCopyDst

	mipCount := len(texturedata.Levels(source))
	if texturedata.IsStorage(source) {
		usage = gputypes.TextureUsageTextureBinding | gputypes.TextureUsageStorageBinding
		mipCount = 1
	}

	image, err := driver.device.CreateTexture(
		&wgpu.TextureDescriptor{
			Usage:         usage,
			Dimension:     gputypes.TextureDimension2D,
			Size:          size,
			Format:        format,
			MipLevelCount: uint32(mipCount),
			SampleCount:   1,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("create material texture: %w", err)
	}

	if err := driver.uploadTextureMips(image, source); err != nil {
		image.Release()

		return nil, err
	}

	view, err := image.CreateView(nil)
	if err != nil {
		image.Release()

		return nil, fmt.Errorf("create material texture view: %w", err)
	}

	bytes := texturedata.ByteSize(source)
	driver.textures[source] = gpuTexture{image: image, view: view, bytes: bytes}
	driver.textureBytes += bytes

	return view, nil
}

func (driver *Driver) uploadTextureMips(image *wgpu.Texture, source *texturedata.Texture) error {
	width, height := source.Size()
	encoding := texturedata.Encoding(source)

	for level, pixels := range texturedata.Levels(source) {
		levelWidth, levelHeight := max(width>>level, 1), max(height>>level, 1)
		rowBytes, rows := texturedata.Layout(encoding, levelWidth, levelHeight)
		levelWidth, levelHeight = texturedata.CopyExtent(encoding, levelWidth, levelHeight)
		//nolint:gosec // Admitted dimensions are at most 4096, including block-rounded mip extents.
		levelSize := gputypes.Extent3D{Width: uint32(levelWidth), Height: uint32(levelHeight), DepthOrArrayLayers: 1}
		//nolint:gosec // Tightly packed rows are bounded by 4096 RGBA pixels; compressed rows are smaller.
		layout := wgpu.ImageDataLayout{BytesPerRow: uint32(rowBytes), RowsPerImage: uint32(rows)}
		if err := driver.queue.WriteTexture(
			&wgpu.ImageCopyTexture{Texture: image, MipLevel: uint32(level)}, pixels, &layout, &levelSize,
		); err != nil {
			return fmt.Errorf("upload texture mip: %w", err)
		}
	}

	return nil
}

// UnloadTexture retires cached bindings and the texture's GPU references.
func (driver *Driver) UnloadTexture(source *texturedata.Texture) {
	driver.clearDecalAtlas(source)

	for id, entry := range driver.materials {
		if entry.albedo == source || entry.normal == source || entry.occlusion == source || entry.reflection == source ||
			entry.lightmap == source || entry.rnm[0] == source || entry.rnm[1] == source || entry.rnm[2] == source {
			entry.release()
			delete(driver.materials, id)
		}
	}

	if cached, ok := driver.textures[source]; ok {
		cached.view.Release()
		cached.image.Release()
		driver.textureBytes -= cached.bytes
		delete(driver.textures, source)
	}
}

func (entry *gpuMaterial) release() {
	if entry.group != nil {
		entry.group.Release()
	}

	entry.uniform.Release()

	if entry.impostor != nil {
		entry.impostor.release()
	}
}

func (driver *Driver) pruneMaterials() {
	for id, entry := range driver.materials {
		if !entry.seen {
			entry.release()
			delete(driver.materials, id)
		}

		entry.seen = false
	}
}

func (driver *Driver) releaseMaterials() {
	driver.closeDecals()

	for id, entry := range driver.materials {
		entry.release()
		delete(driver.materials, id)
	}

	for source := range driver.textures {
		driver.UnloadTexture(source)
	}

	if driver.frameUniform != nil {
		driver.frameUniform.Release()
		driver.frameUniform = nil
	}

	if driver.sampler != nil {
		driver.sampler.Release()
	}

	if driver.lightmapSampler != nil {
		driver.lightmapSampler.Release()
	}

	if driver.impostorSampler != nil {
		driver.impostorSampler.Release()
	}
}

func (entry *gpuMaterial) bindingsChanged(
	albedo, normal, occlusion, reflection, lightmap *texturedata.Texture,
	rnm [3]*texturedata.Texture,
) bool {
	return entry.group == nil || entry.albedo != albedo || entry.normal != normal || entry.occlusion != occlusion ||
		entry.reflection != reflection ||
		entry.lightmap != lightmap || entry.rnm != rnm
}
