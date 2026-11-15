//go:build !linux && !js

package features

import (
	"errors"

	"github.com/karty-game/karty-webgpu/internal/platform"
	"github.com/karty-game/karty-webgpu/pkg/engine"
)

var errPlatform = errors.New("engine features: native platform backend is unavailable")

func unsupported(kind platform.FeatureKind) engine.Feature {
	return platform.Feature{Kind: kind, Install: func(any, platform.FeatureConfig) error { return errPlatform }}
}
func standard() engine.Feature     { return unsupported(platform.StandardFeature) }
func source() engine.Feature       { return unsupported(platform.SourceFeature) }
func impostors() engine.Feature    { return unsupported(platform.ImpostorFeature) }
func pointShadows() engine.Feature { return unsupported(platform.PointShadowFeature) }
func sprites() engine.Feature      { return unsupported(platform.SpriteFeature) }
func compute() engine.Feature      { return unsupported(platform.ComputeFeature) }
