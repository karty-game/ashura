//go:build !linux && !js

package features

import (
	"errors"

	"github.com/karty-game/ashura/internal/platform"
	"github.com/karty-game/ashura/pkg/engine"
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

func clusteredLights() engine.Feature { return unsupported(platform.ClusteredLightsFeature) }

func particles() engine.Feature { return unsupported(platform.ParticleFeature) }

func decals() engine.Feature { return unsupported(platform.DecalFeature) }
