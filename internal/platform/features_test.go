package platform

import (
	"errors"
	"slices"
	"testing"
)

func TestFeatureInstallationUsesExplicitConstructorsInDependencyOrder(t *testing.T) {
	t.Parallel()

	var calls []FeatureKind

	feature := func(kind FeatureKind) Feature {
		return Feature{Kind: kind, Install: func(any, FeatureConfig) error {
			calls = append(calls, kind)

			return nil
		}}
	}

	registered := []Feature{feature(ComputeFeature), feature(SourceFeature), feature(StandardFeature), feature(SpriteFeature)}
	if _, err := ValidateFeatures(0, registered); err != nil {
		t.Fatal(err)
	}

	if err := InstallMaterialFeatures(nil, registered, FeatureConfig{}); err != nil {
		t.Fatal(err)
	}

	if !slices.Equal(calls, []FeatureKind{StandardFeature, SourceFeature}) {
		t.Fatal("material constructors not isolated or ordered", calls)
	}

	if err := InstallOptionalFeatures(nil, registered, FeatureConfig{}); err != nil {
		t.Fatal(err)
	}

	if !slices.Equal(calls, []FeatureKind{StandardFeature, SourceFeature, SpriteFeature, ComputeFeature}) {
		t.Fatal(calls)
	}
}

func TestFeatureConfigRejectsShadowDependenciesBeforeInitialization(t *testing.T) {
	t.Parallel()

	install := func(any, FeatureConfig) error {
		t.Error("config validation invoked a constructor")

		return nil
	}
	for _, config := range []Config{
		{PointShadowResolution: 256},
		{Features: []Feature{{Kind: PointShadowFeature, Install: install}}},
	} {
		if err := ValidateConfigFeatures(config); !errors.Is(err, ErrFeatureMissing) {
			t.Fatal("invalid dependencies admitted", err)
		}
	}
}
