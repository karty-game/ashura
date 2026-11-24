//go:build js && wasm

package uiapp

import (
	"github.com/karty-game/ashura/internal/platform/browser"
	"github.com/karty-game/ashura/internal/uihost"
)

func (attached *attachment) AttachUI(host any) (uihost.Bridge, error) {
	return browser.AttachUI(host, attached.View)
}
