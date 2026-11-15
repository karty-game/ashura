//go:build js && wasm

package engine

import (
	"github.com/karty-game/karty-webgpu/internal/platform"
	"github.com/karty-game/karty-webgpu/internal/platform/browser"
)

func newDriver(config platform.Config) (platform.Driver, error) { return browser.New(config) }
