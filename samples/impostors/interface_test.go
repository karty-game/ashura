package main

import (
	"errors"
	"testing"

	"github.com/karty-game/karty-webgpu/pkg/engine"
	"github.com/karty-game/karty-webgpu/pkg/impostor"
)

var errComparisonUI = errors.New("retained UI update failed")

func TestQualityModesRestoreAdaptiveWork(t *testing.T) {
	t.Parallel()

	presentation := &comparisonPresentation{quality: impostor.DefaultQuality()}
	controls := &comparisonUI{presentation: presentation, mode: qualityLow, farText: "16", nearText: "128", mipBias: -.5}

	low, err := controls.settings()
	if err != nil || low.Adaptive || low.MaxCorrections != 0 || low.MipBias != -.5 {
		t.Fatalf("low quality = %+v, error %v", low, err)
	}

	presentation.quality = low
	controls.mode = qualityAuto

	adaptive, err := controls.settings()
	if err != nil || !adaptive.Adaptive || adaptive.MaxCorrections != 2 || adaptive.MipBias != low.MipBias {
		t.Fatalf("restored Auto must restore both correction ramps: %+v, error %v", adaptive, err)
	}
}

func TestQualityInputPreservesLastAcceptedSettings(t *testing.T) {
	t.Parallel()

	accepted := impostor.DefaultQuality()
	presentation := &comparisonPresentation{quality: accepted}

	controls := &comparisonUI{presentation: presentation, mode: qualityAuto, nearText: "128"}
	for _, value := range []string{"", "invalid", "NaN", "+Inf", "0", "65"} {
		controls.farText = value
		if _, err := controls.settings(); err == nil {
			t.Errorf("invalid Far %q accepted", value)
		}

		if presentation.quality != accepted {
			t.Fatal("invalid input replaced accepted settings")
		}
	}

	controls.farText = "24"

	settings, err := controls.settings()
	if err != nil || settings.FarPixels != 24 || settings.NearPixels != 128 {
		t.Fatalf("corrected input did not recover: %+v, error %v", settings, err)
	}
}

func TestPresentationFailureReachesSceneUpdate(t *testing.T) {
	t.Parallel()

	expected := errComparisonUI
	presentation := &comparisonPresentation{}
	presentation.failures.Check(expected)

	comparison := comparisonFixture()

	comparison.prepare = presentation.update
	if err := comparison.update(engine.Input{}, 0); !errors.Is(err, expected) {
		t.Fatalf("presentation failure did not reach update caller: %v", err)
	}
}
