//go:build js && wasm

package browser

import (
	"errors"
	"fmt"
)

var errDeviceLost = errors.New("browser WebGPU device was lost")

// watchDevice owns its promise callbacks until loss or explicit device destruction.
// It sleeps on a promise rather than polling or waiting during frame submission.
func (driver *Driver) watchDevice() {
	defer close(driver.deviceWatchDone)

	information, err := awaitPromise(driver.device.Get("lost"))
	driver.mu.Lock()
	if driver.closed {
		driver.mu.Unlock()

		return
	}

	if err != nil {
		driver.deviceErr = fmt.Errorf("%w: %w", errDeviceLost, err)
	} else {
		driver.deviceErr = fmt.Errorf("%w (%s): %s", errDeviceLost, information.Get("reason").String(), information.Get("message").String())
	}

	failure := driver.deviceErr
	running := driver.running
	driver.mu.Unlock()

	if running {
		driver.cancelFrame()
		driver.finish(failure)
	}
}

func (driver *Driver) deviceFailure() error {
	driver.mu.Lock()
	defer driver.mu.Unlock()

	return driver.deviceErr
}
