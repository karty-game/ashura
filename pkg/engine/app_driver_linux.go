//go:build linux && !js

package engine

import (
	"github.com/karty-game/ashura/internal/platform"
	"github.com/karty-game/ashura/internal/platform/native"
)

func newDriver(config platform.Config) (platform.Driver, error) { return native.New(config) }
