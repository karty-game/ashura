package platform

import (
	"errors"
	"fmt"
)

type FeatureKind uint8

const (
	StandardFeature FeatureKind = iota + 1
	SourceFeature
	ImpostorFeature
	PointShadowFeature
	SpriteFeature
	ComputeFeature
)

var (
	ErrFeatureMissing   = errors.New("engine feature is not registered")
	ErrFeatureInvalid   = errors.New("invalid engine feature registration")
	ErrFeatureDuplicate = errors.New("engine feature is already registered")
)

// Feature selects an implementation through an explicit frontend constructor.
// The core never selects a concrete implementation from a runtime feature flag.
type Feature struct {
	Kind    FeatureKind
	Install func(host any, config FeatureConfig) error
}

type FeatureConfig struct{ PointShadowResolution uint32 }
type FeatureMask uint8

func (mask FeatureMask) Has(kind FeatureKind) bool {
	return kind >= StandardFeature && kind <= ComputeFeature && mask&(1<<(kind-1)) != 0
}

// ValidateFeatures checks the complete registration batch before mutation.
func ValidateFeatures(existing FeatureMask, features []Feature) (FeatureMask, error) {
	mask := existing

	for _, feature := range features {
		if feature.Kind < StandardFeature || feature.Kind > ComputeFeature || feature.Install == nil {
			return existing, ErrFeatureInvalid
		}

		if mask.Has(feature.Kind) {
			return existing, fmt.Errorf("%s: %w", feature.Kind, ErrFeatureDuplicate)
		}

		mask |= 1 << (feature.Kind - 1)
	}

	return mask, nil
}

// ValidateConfigFeatures checks cross-feature requirements before opening a GPU.
func ValidateConfigFeatures(config Config) error {
	mask, err := ValidateFeatures(0, config.Features)
	if err != nil {
		return err
	}

	if config.PointShadowResolution != 0 && !mask.Has(PointShadowFeature) {
		return fmt.Errorf("%s: %w", PointShadowFeature, ErrFeatureMissing)
	}

	if mask.Has(PointShadowFeature) && !mask.Has(StandardFeature) && !mask.Has(SourceFeature) {
		return fmt.Errorf("point shadows require a Standard or Source renderer: %w", ErrFeatureMissing)
	}

	return nil
}

func (kind FeatureKind) String() string {
	switch kind {
	case StandardFeature:
		return "standard materials"
	case SourceFeature:
		return "Source materials"
	case ImpostorFeature:
		return "impostors"
	case PointShadowFeature:
		return "point shadows"
	case SpriteFeature:
		return "sprites"
	case ComputeFeature:
		return "compute"
	}

	return "unknown feature"
}

// Materials precede shared shadow sampling; other optional implementations follow.
func InstallMaterialFeatures(host any, features []Feature, config FeatureConfig) error {
	return installFeatures(host, features, config, StandardFeature, ImpostorFeature)
}

func InstallOptionalFeatures(host any, features []Feature, config FeatureConfig) error {
	return installFeatures(host, features, config, PointShadowFeature, ComputeFeature)
}

func installFeatures(host any, features []Feature, config FeatureConfig, first, last FeatureKind) error {
	for kind := first; kind <= last; kind++ {
		for _, feature := range features {
			if feature.Kind == kind {
				if err := feature.Install(host, config); err != nil {
					return fmt.Errorf("initialize %s: %w", kind, err)
				}
			}
		}
	}

	return nil
}
