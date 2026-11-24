//go:build js

package lightmapbuild

import (
	"context"
	"errors"

	"github.com/karty-game/ashura/pkg/lightmap"
)

// BrowserOptions selects the host Chromium executable for offline execution.
type BrowserOptions struct{ Executable string }

var errBrowserHost = errors.New("offline browser bake requires a host process")

// BakeBrowser is a host build operation; a browser game uses lightmap.New.
func BakeBrowser(_ context.Context, _ *lightmap.Asset, _ lightmap.Point, _ BrowserOptions) (Result, error) {
	return Result{}, errBrowserHost
}
