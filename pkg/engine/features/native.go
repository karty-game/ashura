//go:build linux && !js

package features

import (
	"github.com/karty-game/karty-webgpu/internal/platform"
	"github.com/karty-game/karty-webgpu/internal/platform/native"
	"github.com/karty-game/karty-webgpu/pkg/engine"
)

func standard() engine.Feature {
	return platform.Feature{
		Kind:    platform.StandardFeature,
		Install: func(host any, _ platform.FeatureConfig) error { return native.RegisterStandard(host) },
	}
}
func source() engine.Feature {
	return platform.Feature{
		Kind:    platform.SourceFeature,
		Install: func(host any, _ platform.FeatureConfig) error { return native.RegisterSource(host) },
	}
}
func impostors() engine.Feature {
	return platform.Feature{
		Kind:    platform.ImpostorFeature,
		Install: func(host any, _ platform.FeatureConfig) error { return native.RegisterImpostors(host) },
	}
}
func pointShadows() engine.Feature {
	return platform.Feature{Kind: platform.PointShadowFeature, Install: func(host any, config platform.FeatureConfig) error {
		return native.RegisterPointShadows(host, config.PointShadowResolution)
	}}
}
func sprites() engine.Feature {
	return platform.Feature{
		Kind:    platform.SpriteFeature,
		Install: func(host any, _ platform.FeatureConfig) error { return native.RegisterSprites(host) },
	}
}
func compute() engine.Feature {
	return platform.Feature{
		Kind:    platform.ComputeFeature,
		Install: func(host any, _ platform.FeatureConfig) error { return native.RegisterCompute(host) },
	}
}
