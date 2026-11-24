package engine

import (
	"fmt"

	"github.com/karty-game/ashura/internal/platform"
	"github.com/karty-game/ashura/internal/render"
)

// Feature is an explicit engine implementation selected from pkg/engine/features.
// UI is attached separately through pkg/ui/app, with its own retained View.
type Feature = platform.Feature

var errClusterLightCapacity = fmt.Errorf("engine: clustered shading supports at most %d lights", render.ClusterLightLimit)

var (
	ErrFeatureMissing   = platform.ErrFeatureMissing
	ErrFeatureInvalid   = platform.ErrFeatureInvalid
	ErrFeatureDuplicate = platform.ErrFeatureDuplicate
)

// Register selects implementations before Run. The complete batch is admitted
// before changes are applied; duplicates and invalid features leave it unchanged.
func (app *App) Register(features ...Feature) error {
	if app == nil || app.closed {
		return errAppClosed
	}

	if app.driver != nil {
		return errAppRunning
	}

	mask, err := platform.ValidateFeatures(app.featureMask, features)
	if err != nil {
		return err
	}

	app.features = append(app.features, features...)
	app.featureMask = mask

	return nil
}

func (app *App) validateFeatureSetup(shadowResolution uint32) error {
	if shadowResolution != 0 && !app.featureMask.Has(platform.PointShadowFeature) {
		return missingFeature(platform.PointShadowFeature)
	}

	if app.screen2D != nil && app.screen2D.Stats().Sprites > 0 && !app.featureMask.Has(platform.SpriteFeature) {
		return missingFeature(platform.SpriteFeature)
	}

	if len(app.particles) != 0 && !app.featureMask.Has(platform.ParticleFeature) {
		return missingFeature(platform.ParticleFeature)
	}

	if (len(app.buffers) > 0 || len(app.computePipelines) > 0 || len(app.computeGroups) > 0) &&
		!app.featureMask.Has(platform.ComputeFeature) {
		return missingFeature(platform.ComputeFeature)
	}

	return nil
}

func missingFeature(kind platform.FeatureKind) error {
	return fmt.Errorf("register %s before Run: %w", kind, ErrFeatureMissing)
}

func (app *App) requireDrawFeature(draw render.DrawCall) error {
	kind := platform.StandardFeature
	if draw.Impostor != nil {
		kind = platform.ImpostorFeature
	} else if draw.Source {
		kind = platform.SourceFeature
	}

	if !app.featureMask.Has(kind) {
		return missingFeature(kind)
	}

	return nil
}

// SetLightCulling switches the registered clustered lighting renderer between
// compute culling and an all-lights loop for visual and performance comparisons.
// Call it on the App loop, or before Run.
func (app *App) SetLightCulling(enabled bool) { app.unculledLights = !enabled }
