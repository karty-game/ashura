//go:build !linux && !js

package engine

import (
	"errors"

	"github.com/karty-game/karty-webgpu/internal/platform"
)

func newDriver(platform.Config) (platform.Driver, error) {
	return nil, errors.New("engine: native WebGPU driver is not available on this operating system")
}
