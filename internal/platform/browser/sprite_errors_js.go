//go:build js && wasm

package browser

import (
	"errors"
	"fmt"
	"syscall/js"
)

var (
	errSpriteLimit = errors.New("Screen2D records exceed this device's storage limits")
	errSpriteGPU   = errors.New("Screen2D GPU resource creation failed")
)

// Scopes run only for new device resources. Their promise callbacks report failures
// through Run without waiting inside an animation callback or polling the GPU.
func (driver *spriteFeature) beginSpriteResources() func() {
	driver.device.Call("pushErrorScope", "out-of-memory")
	driver.device.Call("pushErrorScope", "validation")

	return func() {
		driver.watchSpriteResourceError(driver.device.Call("popErrorScope"))
		driver.watchSpriteResourceError(driver.device.Call("popErrorScope"))
	}
}

func (driver *spriteFeature) watchSpriteResourceError(promise js.Value) {
	var resolved, rejected js.Func

	resolved = js.FuncOf(func(_ js.Value, args []js.Value) any {
		defer resolved.Release()
		defer rejected.Release()

		if len(args) > 0 && !args[0].IsNull() && !args[0].IsUndefined() {
			driver.failSpriteResources(args[0].Get("message").String())
		}

		return nil
	})
	rejected = js.FuncOf(func(_ js.Value, args []js.Value) any {
		defer resolved.Release()
		defer rejected.Release()

		message := "resource error scope rejected"
		if len(args) > 0 {
			message = args[0].String()
		}

		driver.failSpriteResources(message)

		return nil
	})
	promise.Call("then", resolved, rejected)
}

func (driver *spriteFeature) failSpriteResources(message string) {
	driver.mu.Lock()
	if driver.closed {
		driver.mu.Unlock()

		return
	}

	if driver.deviceErr == nil {
		driver.deviceErr = fmt.Errorf("%w: %s", errSpriteGPU, message)
	}

	failure, running := driver.deviceErr, driver.running
	driver.mu.Unlock()

	if running {
		driver.cancelFrame()
		driver.finish(failure)
	}
}
