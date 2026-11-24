//go:build linux

package native

import (
	"github.com/karty-game/ashura/internal/platform"
	"github.com/karty-game/ashura/internal/render"
	wgpu "github.com/karty-game/ashura/internal/webgpu"
)

type shadowBackend interface {
	Record(encoder *wgpu.CommandEncoder, frame render.Frame) error
	DrawDebug(pass *wgpu.RenderPassEncoder, frame render.Frame)
	Close()
}

type pointShadowGPU struct {
	*gpuPointShadow

	debugPipeline *wgpu.RenderPipeline
	debugGroup    *wgpu.BindGroup
	state         render.PointShadowState
	faces         [render.PointShadowFaces]*wgpu.TextureView
	pipelines     [2]*wgpu.RenderPipeline
	uniforms      [render.PointShadowFaces]*wgpu.Buffer
	groups        [2][render.PointShadowFaces]*wgpu.BindGroup
	faceData      [render.PointShadowFaces][render.PointShadowFaceSize]byte
}

type pointShadowFeature struct {
	*Driver

	pointShadow *pointShadowGPU
}

// RegisterPointShadows installs the explicit pre-Run shadow passes and debug view.
func RegisterPointShadows(host any, resolution uint32) error {
	driver, ok := host.(*Driver)
	if !ok || driver == nil || driver.closed || driver.device == nil {
		return platform.ErrFeatureInvalid
	}

	if driver.shadowBridge != nil {
		return platform.ErrFeatureDuplicate
	}

	if driver.pointShadow == nil {
		return platform.ErrFeatureMissing
	}

	if err := driver.preparePointShadowSampling(resolution); err != nil {
		return err
	}

	feature := &pointShadowFeature{Driver: driver, pointShadow: &pointShadowGPU{gpuPointShadow: driver.pointShadow}}
	driver.shadowBridge = feature

	if resolution == 0 {
		return nil
	}

	if err := feature.initPointShadowPasses(feature.pointShadow); err != nil {
		return err
	}

	return feature.initPointShadowDebug()
}

func (feature *pointShadowFeature) Record(encoder *wgpu.CommandEncoder, frame render.Frame) error {
	return feature.recordPointShadow(encoder, frame)
}

func (feature *pointShadowFeature) DrawDebug(pass *wgpu.RenderPassEncoder, frame render.Frame) {
	feature.drawPointShadowDebug(pass, frame)
}

func (feature *pointShadowFeature) Close() {
	if feature.pointShadow != nil {
		feature.pointShadow.release()
		feature.pointShadow = nil
	}
}

func (driver *Driver) recordPointShadow(encoder *wgpu.CommandEncoder, frame render.Frame) error {
	if driver.shadowBridge != nil {
		return driver.shadowBridge.Record(encoder, frame)
	}

	if _, enabled := render.ShadowLight(frame); enabled {
		return platform.ErrFeatureMissing
	}

	return nil
}

func (driver *Driver) drawPointShadowDebug(pass *wgpu.RenderPassEncoder, frame render.Frame) {
	if driver.shadowBridge != nil {
		driver.shadowBridge.DrawDebug(pass, frame)
	}
}

func (shadow *pointShadowGPU) release() {
	if shadow.debugGroup != nil {
		shadow.debugGroup.Release()
	}

	if shadow.debugPipeline != nil {
		shadow.debugPipeline.Release()
	}

	for variant := range 2 {
		for _, group := range shadow.groups[variant] {
			if group != nil {
				group.Release()
			}
		}

		if shadow.pipelines[variant] != nil {
			shadow.pipelines[variant].Release()
		}
	}

	for face := range render.PointShadowFaces {
		if shadow.faces[face] != nil {
			shadow.faces[face].Release()
		}

		if shadow.uniforms[face] != nil {
			shadow.uniforms[face].Release()
		}
	}
}
