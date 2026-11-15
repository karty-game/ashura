//go:build linux

package native

import (
	"fmt"

	"github.com/gogpu/gputypes"
	wgpu "github.com/karty-game/karty-webgpu/internal/webgpu"
)

func (driver *Driver) resizeTargets() error {
	if driver.depthView != nil && driver.depthWidth == driver.width && driver.depthHeight == driver.height {
		return nil
	}

	depth, depthView, err := driver.createAttachment(gputypes.TextureFormatDepth24Plus)
	if err != nil {
		return err
	}

	var (
		color     *wgpu.Texture
		colorView *wgpu.TextureView
	)
	if driver.sampleCount > 1 {
		color, colorView, err = driver.createAttachment(driver.format)
		if err != nil {
			depthView.Release()
			depth.Release()

			return err
		}
	}

	driver.releaseTargets()
	driver.depthTexture, driver.depthView = depth, depthView
	driver.colorTexture, driver.colorView = color, colorView
	driver.depthWidth, driver.depthHeight = driver.width, driver.height

	return nil
}

func (driver *Driver) createAttachment(format gputypes.TextureFormat) (*wgpu.Texture, *wgpu.TextureView, error) {
	texture, err := driver.device.CreateTexture(
		&wgpu.TextureDescriptor{
			Usage:         gputypes.TextureUsageRenderAttachment,
			Dimension:     gputypes.TextureDimension2D,
			Size:          gputypes.Extent3D{Width: driver.width, Height: driver.height, DepthOrArrayLayers: 1},
			Format:        format,
			MipLevelCount: 1,
			SampleCount:   driver.sampleCount,
		},
	)
	if err != nil {
		return nil, nil, fmt.Errorf("create render attachment: %w", err)
	}

	view, err := texture.CreateView(nil)
	if err != nil {
		texture.Release()

		return nil, nil, fmt.Errorf("create attachment view: %w", err)
	}

	return texture, view, nil
}

func (driver *Driver) colorAttachment(surface *wgpu.TextureView, clearColor [4]float32) wgpu.RenderPassColorAttachment {
	attachment := wgpu.RenderPassColorAttachment{
		View:       surface,
		LoadOp:     gputypes.LoadOpClear,
		StoreOp:    gputypes.StoreOpStore,
		ClearValue: wgpu.Color{R: float64(clearColor[0]), G: float64(clearColor[1]), B: float64(clearColor[2]), A: float64(clearColor[3])},
	}
	if driver.sampleCount > 1 {
		attachment.View = driver.colorView
		attachment.ResolveTarget = surface
		attachment.StoreOp = gputypes.StoreOpDiscard
	}

	return attachment
}

func (driver *Driver) releaseTargets() {
	if driver.depthView != nil {
		driver.depthView.Release()
		driver.depthView = nil
	}

	if driver.depthTexture != nil {
		driver.depthTexture.Release()
		driver.depthTexture = nil
	}

	if driver.colorView != nil {
		driver.colorView.Release()
		driver.colorView = nil
	}

	if driver.colorTexture != nil {
		driver.colorTexture.Release()
		driver.colorTexture = nil
	}
}
