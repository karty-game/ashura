//go:build js && wasm

package browser

import (
	"context"
	"fmt"
	"sync"
	"syscall/js"
	"time"

	"github.com/karty-game/karty-webgpu/internal/platform"
	"github.com/karty-game/karty-webgpu/internal/render"
)

// Run schedules frame updates through requestAnimationFrame until ctx is
// canceled, the driver closes, or the frame callback returns an error.
func (driver *Driver) Run(ctx context.Context, update func(time.Duration, platform.InputState) (render.Frame, error)) error {
	driver.mu.Lock()
	if driver.closed {
		driver.mu.Unlock()

		return errDriverClosed
	}

	if driver.deviceErr != nil {
		failure := driver.deviceErr
		driver.mu.Unlock()

		return failure
	}

	if driver.running {
		driver.mu.Unlock()

		return errDriverRunning
	}

	driver.running = true
	driver.done = make(chan struct{})
	driver.lastFrameAt = time.Now()
	driver.finishOnce = sync.Once{}
	driver.runErr = nil
	driver.mu.Unlock()

	driver.frame = js.FuncOf(func(js.Value, []js.Value) any {
		err := driver.step(update)
		if err == nil {
			err = driver.deviceFailure()
		}

		if driver.closed || err != nil {
			driver.finish(err)

			return nil
		}

		driver.frameID = driver.window.Call("requestAnimationFrame", driver.frame)

		return nil
	})
	driver.frameID = driver.window.Call("requestAnimationFrame", driver.frame)

	select {
	case <-ctx.Done():
		driver.cancelFrame()
		driver.frame.Release()

		return ctx.Err()
	case <-driver.done:
		driver.mu.Lock()
		err := driver.runErr
		driver.mu.Unlock()
		driver.cancelFrame()

		if driver.closed && driver.deviceWatchDone != nil {
			<-driver.deviceWatchDone
		}

		driver.frame.Release()

		return err
	}
}

func (driver *Driver) finish(err error) {
	driver.finishOnce.Do(func() {
		driver.mu.Lock()
		driver.runErr = err
		done := driver.done
		driver.mu.Unlock()
		close(done)
	})
}

func (driver *Driver) cancelFrame() {
	window := driver.window
	if !window.IsUndefined() && window.Get("cancelAnimationFrame").Type() == js.TypeFunction {
		window.Call("cancelAnimationFrame", driver.frameID)
	}

	driver.mu.Lock()
	driver.running = false
	driver.mu.Unlock()
}

func (driver *Driver) step(update func(time.Duration, platform.InputState) (render.Frame, error)) error {
	if err := driver.deviceFailure(); err != nil {
		return err
	}

	if driver.closed {
		return nil
	}

	now := time.Now()
	delta := now.Sub(driver.lastFrameAt)
	driver.lastFrameAt = now
	driver.resizeCanvas()
	input := driver.input
	input.Width, input.Height = driver.canvasWidth, driver.canvasHeight
	driver.resetInputDeltas()
	driver.metrics.Begin(delta)

	frame, err := update(delta, input)

	if driver.closed {
		return nil
	}

	if err != nil {
		return fmt.Errorf("browser update: %w", err)
	}

	driver.setPointerLockMode(frame.PointerLock)

	if driver.uiBridge != nil {
		driver.uiBridge.syncInput()
	}

	if err := driver.present(frame); err != nil {
		return fmt.Errorf("browser present: %w", err)
	}

	driver.metrics.Resources(len(driver.textures), driver.textureBytes)
	driver.metrics.Finish()

	return nil
}
