//go:build linux

package native

import (
	"github.com/gogpu/gputypes"
	"github.com/karty-game/ashura/internal/texturedata"
	"github.com/karty-game/ashura/internal/uipaint"
	wgpu "github.com/karty-game/ashura/internal/webgpu"
)

// Prepared page resources are shared by the independent sprite and UI layers.
type uiPage struct {
	used    uint64
	texture *wgpu.Texture
	view    *wgpu.TextureView
	group   *wgpu.BindGroup
}

// UI and sprites share GPU-ready page admission; each supplies its own sampler.
func (driver *Driver) uploadPreparedPage(page *uipaint.Texture, pipeline *wgpu.RenderPipeline, sampler *wgpu.Sampler) (uiPage, error) {
	format, err := driver.preparedPageFormat(page)
	if err != nil {
		return uiPage{}, err
	}

	width := uint32(page.Width)   //nolint:gosec // Prepared image admission bounds dimensions to positive device-compatible values.
	height := uint32(page.Height) //nolint:gosec // Prepared image admission bounds dimensions to positive device-compatible values.
	rowBytes, rows := texturedata.Layout(page.Format, page.Width, page.Height)
	size := gputypes.Extent3D{Width: width, Height: height, DepthOrArrayLayers: 1}

	texture, err := driver.device.CreateTexture(&wgpu.TextureDescriptor{
		Size: size, Dimension: gputypes.TextureDimension2D, Format: format,
		MipLevelCount: 1, SampleCount: 1, Usage: gputypes.TextureUsageTextureBinding | gputypes.TextureUsageCopyDst,
	})
	if err != nil {
		return uiPage{}, err
	}

	if err := driver.queue.WriteTexture(
		&wgpu.ImageCopyTexture{Texture: texture, Aspect: wgpu.TextureAspectAll},
		page.Pixels,
		&wgpu.ImageDataLayout{
			BytesPerRow:  uint32(rowBytes), //nolint:gosec // Admitted dimensions bound row bytes to 16,384.
			RowsPerImage: uint32(rows),     //nolint:gosec // Admitted dimensions bound rows to 4,096.
		},
		&size,
	); err != nil {
		texture.Release()

		return uiPage{}, err
	}

	view, err := texture.CreateView(nil)
	if err != nil {
		texture.Release()

		return uiPage{}, err
	}

	layout := pipeline.GetBindGroupLayout(1)
	defer layout.Release()

	group, err := driver.device.CreateBindGroup(&wgpu.BindGroupDescriptor{
		Layout:  layout,
		Entries: []wgpu.BindGroupEntry{{Binding: 0, TextureView: view}, {Binding: 1, Sampler: sampler}},
	})
	if err != nil {
		view.Release()
		texture.Release()

		return uiPage{}, err
	}

	return uiPage{texture: texture, view: view, group: group}, nil
}

func (driver *Driver) preparedPageFormat(page *uipaint.Texture) (gputypes.TextureFormat, error) {
	if page == nil || !uipaint.ValidPage(page.Width, page.Height, page.Format, len(page.Pixels)) {
		return 0, errNativeCompression
	}

	enabled, format, _ := driver.compression.formats(page.Format)
	if !enabled {
		return 0, errNativeCompression
	}

	return format, nil
}
