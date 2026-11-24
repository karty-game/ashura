//go:build js

package lightmapbuild

import (
	"context"

	"github.com/karty-game/ashura/pkg/lightmap"
)

// RNMSession is a host-only offline worker. Browser games use lightmap.NewRNM.
type RNMSession struct{}

// NewRNMSession requires an installed host Chromium process.
func NewRNMSession(_ context.Context, _ BrowserOptions) (*RNMSession, error) {
	return nil, errBrowserHost
}

// Bake is a host operation using the same shader as the runtime baker.
func (*RNMSession) Bake(_ context.Context, _ *lightmap.Asset, _ lightmap.RNMScene) (RNMResult, error) {
	return RNMResult{}, errBrowserHost
}

// Close releases a host-only session.
func (*RNMSession) Close() error { return nil }
