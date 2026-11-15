//go:build linux && !js

package uiapp

import (
	"github.com/karty-game/karty-webgpu/internal/platform/native"
	"github.com/karty-game/karty-webgpu/internal/uihost"
)

func (attached *attachment) AttachUI(host any) (uihost.Bridge, error) {
	return native.AttachUI(host, attached.View)
}
