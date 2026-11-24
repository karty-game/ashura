package engine

import (
	"errors"
	"testing"

	"github.com/karty-game/ashura/internal/impostordata"
	"github.com/karty-game/ashura/internal/platform"
	"github.com/karty-game/ashura/internal/render"
)

func TestFeatureRegistrationIsAtomicAndBeforeRun(t *testing.T) {
	t.Parallel()

	app := NewApp(AppConfig{})
	defer func() { _ = app.Close() }()

	installed := 0

	makeFeature := func(kind platform.FeatureKind) Feature {
		return Feature{Kind: kind, Install: func(any, platform.FeatureConfig) error {
			installed++

			return nil
		}}
	}
	if err := app.Register(makeFeature(platform.StandardFeature)); err != nil {
		t.Fatal(err)
	}

	if err := app.Register(makeFeature(platform.SourceFeature), Feature{}); !errors.Is(err, ErrFeatureInvalid) {
		t.Fatal(err)
	}

	if app.featureMask.Has(platform.SourceFeature) || len(app.features) != 1 || installed != 0 {
		t.Fatal("failed batch changed state or initialized a backend")
	}

	if err := app.Register(
		makeFeature(platform.SourceFeature),
		makeFeature(platform.StandardFeature),
	); !errors.Is(
		err,
		ErrFeatureDuplicate,
	) {
		t.Fatal(err)
	}

	if app.featureMask.Has(platform.SourceFeature) || len(app.features) != 1 {
		t.Fatal("duplicate batch partially registered a feature")
	}

	if err := app.Register(makeFeature(platform.SourceFeature), makeFeature(platform.SourceFeature)); !errors.Is(err, ErrFeatureDuplicate) {
		t.Fatal("in-batch duplicate admitted", err)
	}

	app.driver = &texturePrepareDriver{}
	err := app.Register(makeFeature(platform.SourceFeature))
	app.driver = nil

	if !errors.Is(err, errAppRunning) {
		t.Fatal("running application accepted feature", err)
	}

	if err := app.Close(); err != nil {
		t.Fatal(err)
	}

	if err := app.Register(makeFeature(platform.SourceFeature)); !errors.Is(err, ErrClosed) {
		t.Fatal("closed application accepted feature", err)
	}
}

func TestMissingFeatureAdmissionDoesNotSelectFallback(t *testing.T) {
	t.Parallel()

	app := NewApp(AppConfig{})
	defer func() { _ = app.Close() }()

	for _, draw := range []render.DrawCall{{}, {Source: true}, {Impostor: &impostordata.Document{}}} {
		if err := app.requireDrawFeature(draw); !errors.Is(err, ErrFeatureMissing) {
			t.Fatal("missing renderer accepted", err)
		}
	}

	if err := app.validateFeatureSetup(256); !errors.Is(err, ErrFeatureMissing) {
		t.Fatal("missing point shadows accepted", err)
	}

	if err := app.Register(
		Feature{Kind: platform.StandardFeature, Install: func(any, platform.FeatureConfig) error { return nil }},
	); err != nil {
		t.Fatal(err)
	}

	if err := app.requireDrawFeature(render.DrawCall{}); err != nil {
		t.Fatal("registered standard renderer rejected", err)
	}

	if err := app.requireDrawFeature(render.DrawCall{Source: true}); !errors.Is(err, ErrFeatureMissing) {
		t.Fatal("Source draw fell back to Standard", err)
	}
}
