//go:build js

package lightmapbuild

import (
	"context"

	"github.com/karty-game/karty-webgpu/pkg/lightmap"
)

// BakeRNMBrowser is a host build operation; browser games use lightmap.NewRNM.
func BakeRNMBrowser(_ context.Context, _ *lightmap.Asset, _ lightmap.RNMScene, _ BrowserOptions) (RNMResult, error) {
	return RNMResult{}, errBrowserHost
}
