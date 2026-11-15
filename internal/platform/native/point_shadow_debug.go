//go:build linux

package native

import (
	"github.com/gogpu/gputypes"
	"github.com/karty-game/karty-webgpu/internal/render"
	wgpu "github.com/karty-game/karty-webgpu/internal/webgpu"
)

func (driver *pointShadowFeature) initPointShadowDebug() error {
	shadow := driver.pointShadow
	if shadow.debugPipeline != nil {
		return nil
	}

	shader, err := driver.device.CreateShaderModuleWGSL(render.PointShadowDebugShader())
	if err != nil {
		return err
	}
	defer shader.Release()

	shadow.debugPipeline, err = driver.device.CreateRenderPipeline(
		&wgpu.RenderPipelineDescriptor{
			Label:  "point shadow debug faces",
			Vertex: wgpu.VertexState{Module: shader, EntryPoint: pointShadowVertexEntry},
			Fragment: &wgpu.FragmentState{
				Module: shader,
				EntryPoint: render.FragmentEntry(
					driver.format != gputypes.TextureFormatBGRA8UnormSrgb && driver.format != gputypes.TextureFormatRGBA8UnormSrgb,
				),
				Targets: []wgpu.ColorTargetState{{Format: driver.format, WriteMask: gputypes.ColorWriteMaskAll}},
			},
			Primitive: wgpu.PrimitiveState{
				Topology:  gputypes.PrimitiveTopologyTriangleList,
				FrontFace: gputypes.FrontFaceCCW,
				CullMode:  gputypes.CullModeNone,
			},
			DepthStencil: &wgpu.DepthStencilState{Format: gputypes.TextureFormatDepth24Plus, DepthCompare: gputypes.CompareFunctionAlways},
			Multisample:  wgpu.MultisampleState{Count: driver.sampleCount, Mask: allSampleMask},
		},
	)
	if err != nil {
		return err
	}

	layout := shadow.debugPipeline.GetBindGroupLayout(0)
	defer layout.Release()

	shadow.debugGroup, err = driver.device.CreateBindGroup(
		&wgpu.BindGroupDescriptor{
			Layout:  layout,
			Entries: []wgpu.BindGroupEntry{{Binding: 0, TextureView: shadow.cube}, {Binding: 1, Sampler: shadow.sampler}},
		},
	)

	return err
}

func (driver *pointShadowFeature) drawPointShadowDebug(pass *wgpu.RenderPassEncoder, frame render.Frame) {
	light, enabled := render.ShadowLight(frame)
	if !enabled || !light.DebugShadowFaces {
		return
	}

	pass.SetPipeline(driver.pointShadow.debugPipeline)
	pass.SetBindGroup(0, driver.pointShadow.debugGroup, nil)
	pass.Draw(3, 1, 0, 0)
}
