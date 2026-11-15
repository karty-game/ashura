//go:build linux

package native

import (
	"github.com/gogpu/gputypes"
	wgpu "github.com/karty-game/karty-webgpu/internal/webgpu"
)

// Empty worlds clear the drawable directly without allocating world depth/MSAA.
func (driver *Driver) clearSurface(encoder *wgpu.CommandEncoder, view *wgpu.TextureView, color [4]float32) error {
	if color == ([4]float32{}) {
		color[3] = 1
	}

	attachment := &driver.passColors[0]
	*attachment = wgpu.RenderPassColorAttachment{
		View: view, LoadOp: gputypes.LoadOpClear, StoreOp: gputypes.StoreOpStore,
		ClearValue: wgpu.Color{R: float64(color[0]), G: float64(color[1]), B: float64(color[2]), A: float64(color[3])},
	}

	pass, err := encoder.BeginRenderPass(&wgpu.RenderPassDescriptor{ColorAttachments: driver.passColors[:]})
	if err != nil {
		return err
	}

	pass.End()
	pass.Release()

	return nil
}
