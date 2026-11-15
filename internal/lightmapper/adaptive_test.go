//go:build !js

package lightmapper

import (
	"context"
	"errors"
	"testing"
)

type adaptiveFixtureRunner struct {
	Calls          []Options
	FailProduction bool
}

func (runner *adaptiveFixtureRunner) Bake(_ context.Context, _ *PreparedScene, receivers []Receiver, options Options) (Result, error) {
	runner.Calls = append(runner.Calls, options)
	if runner.FailProduction && len(runner.Calls) > 1 {
		return Result{}, context.Canceled
	}

	value := float32(2)
	if len(runner.Calls) == 1 {
		value = 100
	}

	result := Result{Estimates: make([]Estimate, len(receivers))}
	for receiverIndex, receiver := range receivers {
		estimate := Estimate{ReceiverID: receiver.ID, Count: options.Samples, VarianceValid: true, SecondMomentsValid: true}

		for kernel := range 4 {
			for channel := range 3 {
				estimate.Statistics[kernel][channel] = value
				estimate.SecondMoments[kernel][channel] = value * value
			}
		}

		result.Estimates[receiverIndex] = estimate
	}

	return result, nil
}

func TestAdaptiveProductionExcludesPilotAndResumesCommittedGroups(t *testing.T) {
	t.Parallel()

	runner := &adaptiveFixtureRunner{}
	scene := &PreparedScene{hash: "prepared-scene"}
	receivers := []Receiver{{ID: 11}, {ID: 12}}
	policy := AdaptivePolicy{
		PilotSamples: 4,
		Quality:      QualityPolicy{Minimum: 8, Maximum: 128, AbsoluteError: .01, RelativeError: .01, Confidence: 2},
		Strict:       true,
		Attempt:      "attempt1",
	}

	var checkpoint AdaptiveState

	result, state, err := BakeAdaptive(
		t.Context(),
		runner,
		scene,
		receivers,
		Options{Outputs: OutputRGB | OutputRNM, Samples: 32, Seed: 5},
		policy,
		"GPU",
		nil,
		func(snapshot AdaptiveState) error {
			checkpoint = snapshot

			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(runner.Calls) != 2 || runner.Calls[0].Seed == runner.Calls[1].Seed || runner.Calls[0].Samples != 4 ||
		runner.Calls[1].Samples != 8 {
		t.Fatal(runner.Calls)
	}

	if result.Estimates[0].Statistics[0][0] != 2 || !state.Completed[0] {
		t.Fatal(result, state)
	}

	resumed := &adaptiveFixtureRunner{}

	again, _, err := BakeAdaptive(
		t.Context(),
		resumed,
		scene,
		receivers,
		Options{Outputs: OutputRGB | OutputRNM, Samples: 32, Seed: 5},
		policy,
		"GPU",
		&checkpoint,
		nil,
	)
	if err != nil || len(resumed.Calls) != 0 || again.Estimates[0] != result.Estimates[0] {
		t.Fatal(again, resumed.Calls, err)
	}

	checkpoint.ProductionSeed++
	if _, _, err = BakeAdaptive(
		t.Context(),
		resumed,
		scene,
		receivers,
		Options{Outputs: OutputRGB | OutputRNM, Samples: 32, Seed: 5},
		policy,
		"GPU",
		&checkpoint,
		nil,
	); err == nil {
		t.Fatal("accepted incompatible production scramble")
	}
}

func TestAdaptiveCheckpointSurvivesCancelledUncommittedProduction(t *testing.T) {
	t.Parallel()

	runner := &adaptiveFixtureRunner{FailProduction: true}
	scene := &PreparedScene{hash: "prepared-scene"}
	receivers := []Receiver{{ID: 1}}
	policy := AdaptivePolicy{
		PilotSamples: 4,
		Quality:      QualityPolicy{Minimum: 8, Maximum: 128, AbsoluteError: .01, RelativeError: .01, Confidence: 2},
		Attempt:      "attempt1",
	}

	var checkpoint AdaptiveState

	_, _, err := BakeAdaptive(
		t.Context(),
		runner,
		scene,
		receivers,
		Options{Outputs: OutputRGB, Samples: 32},
		policy,
		"GPU",
		nil,
		func(snapshot AdaptiveState) error {
			checkpoint = snapshot

			return nil
		},
	)
	if !errors.Is(err, context.Canceled) || checkpoint.Completed[0] {
		t.Fatal(checkpoint, err)
	}

	resumed := &adaptiveFixtureRunner{}

	_, state, err := BakeAdaptive(
		t.Context(),
		resumed,
		scene,
		receivers,
		Options{Outputs: OutputRGB, Samples: 32},
		policy,
		"GPU",
		&checkpoint,
		nil,
	)
	if err != nil || len(resumed.Calls) != 1 || !state.Completed[0] || resumed.Calls[0].Samples != 8 {
		t.Fatal(state, resumed.Calls, err)
	}
}
