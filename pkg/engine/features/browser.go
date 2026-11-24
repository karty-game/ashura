//go:build js && wasm

package features

import (
	"github.com/karty-game/ashura/internal/platform"
	"github.com/karty-game/ashura/internal/platform/browser"
	"github.com/karty-game/ashura/pkg/engine"
)

func standard() engine.Feature {
	return platform.Feature{
		Kind:    platform.StandardFeature,
		Install: func(host any, _ platform.FeatureConfig) error { return browser.RegisterStandard(host) },
	}
}
func source() engine.Feature {
	return platform.Feature{
		Kind:    platform.SourceFeature,
		Install: func(host any, _ platform.FeatureConfig) error { return browser.RegisterSource(host) },
	}
}
func impostors() engine.Feature {
	return platform.Feature{
		Kind:    platform.ImpostorFeature,
		Install: func(host any, _ platform.FeatureConfig) error { return browser.RegisterImpostors(host) },
	}
}
func pointShadows() engine.Feature {
	return platform.Feature{Kind: platform.PointShadowFeature, Install: func(host any, config platform.FeatureConfig) error {
		return browser.RegisterPointShadows(host, config.PointShadowResolution)
	}}
}
func sprites() engine.Feature {
	return platform.Feature{
		Kind:    platform.SpriteFeature,
		Install: func(host any, _ platform.FeatureConfig) error { return browser.RegisterSprites(host) },
	}
}
func compute() engine.Feature {
	return platform.Feature{
		Kind:    platform.ComputeFeature,
		Install: func(host any, _ platform.FeatureConfig) error { return browser.RegisterCompute(host) },
	}
}

func clusteredLights() engine.Feature {
	return platform.Feature{
		Kind:    platform.ClusteredLightsFeature,
		Install: func(host any, _ platform.FeatureConfig) error { return browser.RegisterClusteredLights(host) },
	}
}

func particles() engine.Feature {
	return platform.Feature{
		Kind:    platform.ParticleFeature,
		Install: func(host any, _ platform.FeatureConfig) error { return browser.RegisterParticles(host) },
	}
}

func decals() engine.Feature {
	return platform.Feature{
		Kind:    platform.DecalFeature,
		Install: func(host any, _ platform.FeatureConfig) error { return browser.RegisterDecals(host) },
	}
}
