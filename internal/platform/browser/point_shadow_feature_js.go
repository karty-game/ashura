//go:build js && wasm

package browser

import (
	"syscall/js"

	"github.com/karty-game/karty-webgpu/internal/platform"
	"github.com/karty-game/karty-webgpu/internal/render"
)

type shadowBackend interface {
	Record(encoder js.Value, frame render.Frame) error
	DrawDebug(pass js.Value, frame render.Frame)
	Close()
}

type pointShadowGPU struct {
	*gpuPointShadow

	debugPipeline, debugGroup js.Value
	state                     render.PointShadowState
	faces, uniforms           [render.PointShadowFaces]js.Value
	pipelines                 [2]js.Value
	groups                    [2][render.PointShadowFaces]js.Value
}

type pointShadowFeature struct {
	*Driver

	pointShadow *pointShadowGPU
}

// RegisterPointShadows installs the explicit pre-Run shadow passes and debug view.
func RegisterPointShadows(host any, resolution uint32) error {
	driver, ok := host.(*Driver)
	if !ok || driver == nil || driver.closed || driver.device.IsUndefined() || driver.device.IsNull() {
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

func (feature *pointShadowFeature) Record(encoder js.Value, frame render.Frame) error {
	return feature.recordPointShadow(encoder, frame)
}

func (feature *pointShadowFeature) DrawDebug(pass js.Value, frame render.Frame) {
	feature.drawPointShadowDebug(pass, frame)
}

func (feature *pointShadowFeature) Close() {
	if feature.pointShadow != nil {
		feature.pointShadow.release()
		feature.pointShadow = nil
	}
}

func (driver *Driver) recordPointShadow(encoder js.Value, frame render.Frame) error {
	if driver.shadowBridge != nil {
		return driver.shadowBridge.Record(encoder, frame)
	}

	if _, enabled := render.ShadowLight(frame); enabled {
		return platform.ErrFeatureMissing
	}

	return nil
}

func (driver *Driver) drawPointShadowDebug(pass js.Value, frame render.Frame) {
	if driver.shadowBridge != nil {
		driver.shadowBridge.DrawDebug(pass, frame)
	}
}

func (shadow *pointShadowGPU) release() {
	for _, uniform := range shadow.uniforms {
		if !uniform.IsUndefined() && !uniform.IsNull() {
			uniform.Call("destroy")
		}
	}
}
