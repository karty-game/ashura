//go:build linux

package native

import (
	"github.com/karty-game/karty-webgpu/internal/platform"
	"github.com/karty-game/karty-webgpu/internal/spritepaint"
	wgpu "github.com/karty-game/karty-webgpu/internal/webgpu"
)

type spriteBackend interface {
	Update(frame *spritepaint.Frame) error
	Draw(encoder *wgpu.CommandEncoder, view *wgpu.TextureView) error
	Close()
}

type spriteFeature struct {
	*Driver

	sprites *spriteGPU
}

// RegisterSprites prepares the explicit sprite feature before the frame loop.
func RegisterSprites(host any) error {
	driver, ok := host.(*Driver)
	if !ok || driver == nil || driver.closed || driver.device == nil {
		return platform.ErrFeatureInvalid
	}

	if driver.spriteBridge != nil {
		return platform.ErrFeatureDuplicate
	}

	feature := &spriteFeature{Driver: driver}
	driver.spriteBridge = feature

	return feature.prepareSprites()
}

func (feature *spriteFeature) Update(frame *spritepaint.Frame) error {
	return feature.updateSprites(frame)
}

func (feature *spriteFeature) Draw(encoder *wgpu.CommandEncoder, view *wgpu.TextureView) error {
	return feature.drawSprites(encoder, view)
}

func (feature *spriteFeature) Close() { feature.closeSprites() }

func (driver *Driver) updateSprites(frame *spritepaint.Frame) error {
	if driver.spriteBridge != nil {
		return driver.spriteBridge.Update(frame)
	}

	if frame != nil && len(frame.Batches) != 0 {
		return platform.ErrFeatureMissing
	}

	return nil
}

func (driver *Driver) drawSprites(encoder *wgpu.CommandEncoder, view *wgpu.TextureView) error {
	if driver.spriteBridge != nil {
		return driver.spriteBridge.Draw(encoder, view)
	}

	return nil
}

func (driver *Driver) closeSprites() {
	if driver.spriteBridge != nil {
		driver.spriteBridge.Close()
		driver.spriteBridge = nil
	}
}
