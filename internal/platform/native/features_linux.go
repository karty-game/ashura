//go:build linux

package native

import "github.com/karty-game/ashura/internal/platform"

func (driver *Driver) prepareFeatures(config platform.Config) error {
	featureConfig := platform.FeatureConfig{PointShadowResolution: config.PointShadowResolution}
	if err := platform.InstallLightingFeature(driver, config.Features, featureConfig); err != nil {
		return err
	}

	if err := platform.InstallMaterialFeatures(driver, config.Features, featureConfig); err != nil {
		return err
	}

	if err := driver.preparePointShadowSampling(config.PointShadowResolution); err != nil {
		return err
	}

	if err := driver.prepareClusterMaterialGroups(); err != nil {
		return err
	}

	return platform.InstallOptionalFeatures(driver, config.Features, featureConfig)
}
