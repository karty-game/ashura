//go:build !linux && !js

package uiapp

import (
	"errors"

	"github.com/karty-game/ashura/internal/uihost"
)

var errUnsupported = errors.New("uiapp: platform backend is unavailable")

func (attached *attachment) AttachUI(any) (uihost.Bridge, error) {
	return nil, errUnsupported
}
