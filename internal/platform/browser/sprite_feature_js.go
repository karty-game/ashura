//go:build js && wasm

package browser

import (
	"syscall/js"

	"github.com/karty-game/ashura/internal/platform"
	"github.com/karty-game/ashura/internal/spritepaint"
)

type spriteBackend interface {
	Update(frame *spritepaint.Frame) error
	Draw(encoder, view js.Value)
	Close()
}

type spriteFeature struct {
	*Driver

	sprites *spriteGPU
}

// RegisterSprites prepares the explicit sprite feature before the frame loop.
func RegisterSprites(host any) error {
	driver, ok := host.(*Driver)
	if !ok || driver == nil || driver.closed || driver.device.IsUndefined() || driver.device.IsNull() {
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

func (feature *spriteFeature) Draw(encoder, view js.Value) { feature.drawSprites(encoder, view) }

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

func (driver *Driver) drawSprites(encoder, view js.Value) {
	if driver.spriteBridge != nil {
		driver.spriteBridge.Draw(encoder, view)
	}
}

func (driver *Driver) closeSprites() {
	if driver.spriteBridge != nil {
		driver.spriteBridge.Close()
		driver.spriteBridge = nil
	}
}
