//go:build !js

package lightmapper

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
)

const AdaptiveSchema = "karty.lightmapper-adaptive@1"

var ErrUnconverged = errors.New("lightmapper: fixed production pass did not meet requested statistical quality")

// AdaptiveRunner uses the same hardware transport entry point as a fixed bake.
// The interface also permits deterministic orchestration tests without a GPU.
type AdaptiveRunner interface {
	Bake(ctx context.Context, scene *PreparedScene, receivers []Receiver, options Options) (Result, error)
}

type AdaptivePolicy struct {
	PilotSamples uint32
	Quality      QualityPolicy
	Strict       bool
	Attempt      string
}

// AdaptiveState is a resumable frozen allocation plus atomic completed receiver
// estimates. A content-addressed artifact store must persist snapshots atomically.
//
//nolint:recvcheck // Immutable snapshots use value receivers; running pilot/production phases mutate their owned state.
type AdaptiveState struct {
	Schema, Identity, SceneHash, ReceiverHash, Backend string
	Options                                            Options
	Policy                                             AdaptivePolicy
	PilotSeed, ProductionSeed                          uint32
	Pilot                                              []Estimate
	Allocation                                         []Allocation
	Completed                                          []bool
	Production                                         []Estimate
	Unconverged                                        []uint64
}

// BakeAdaptive excludes pilot samples from production, freezes every receiver's
// count before production, groups equal counts without changing sample identity,
// and checkpoints only complete GPU batches. Save must complete before scheduling
// the next group; a failed save never marks external artifacts as published.
func BakeAdaptive(
	ctx context.Context,
	runner AdaptiveRunner,
	scene *PreparedScene,
	receivers []Receiver,
	options Options,
	policy AdaptivePolicy,
	backend string,
	resume *AdaptiveState,
	save func(AdaptiveState) error,
) (Result, *AdaptiveState, error) {
	if ctx == nil || runner == nil || scene == nil || len(receivers) == 0 || backend == "" || policy.Attempt == "" ||
		policy.PilotSamples < 2 ||
		!qualityValid(policy.Quality) {
		return Result{}, nil, ErrInvalid
	}

	state, err := newAdaptiveState(scene, receivers, options, policy, backend)
	if err != nil {
		return Result{}, nil, err
	}

	if resume != nil {
		if err = resume.validate(*state, receivers); err != nil {
			return Result{}, nil, err
		}

		snapshot := resume.clone()
		state = &snapshot
	} else {
		if err = state.preparePilot(ctx, runner, scene, receivers, save); err != nil {
			return Result{}, state, err
		}
	}

	if err = state.runProduction(ctx, runner, scene, receivers, save); err != nil {
		return Result{}, state, err
	}

	if err = state.assess(receivers); err != nil {
		return Result{}, state, err
	}

	result := Result{
		Schema:       ResultSchema,
		Algorithm:    Algorithm,
		SceneHash:    scene.Hash(),
		ReceiverHash: state.ReceiverHash,
		Options:      options,
		Estimates:    append([]Estimate(nil), state.Production...),
		Backend:      backend,
	}
	if save != nil {
		if err = save(state.clone()); err != nil {
			return Result{}, state, err
		}
	}

	if policy.Strict && len(state.Unconverged) > 0 {
		return result, state, ErrUnconverged
	}

	return result, state, nil
}

func adaptiveSeed(identity, phase string) uint32 {
	sum := sha256.Sum256([]byte(identity + "/" + phase))

	return binary.LittleEndian.Uint32(sum[:])
}

func (state AdaptiveState) clone() AdaptiveState {
	state.Pilot = append([]Estimate(nil), state.Pilot...)
	state.Allocation = append([]Allocation(nil), state.Allocation...)
	state.Completed = append([]bool(nil), state.Completed...)
	state.Production = append([]Estimate(nil), state.Production...)
	state.Unconverged = append([]uint64(nil), state.Unconverged...)

	return state
}

func (state AdaptiveState) validate(expected AdaptiveState, receivers []Receiver) error {
	if !state.sameIdentity(expected, len(receivers)) {
		return errCheckpoint
	}

	for receiverIndex, receiver := range receivers {
		pilot := state.Pilot[receiverIndex]
		if pilot.ReceiverID != receiver.ID || pilot.Count != state.Policy.PilotSamples {
			return errCheckpoint
		}

		stats, err := EstimateStatistics(pilot)
		if err != nil {
			return err
		}

		allocation, err := AllocateProduction(stats, state.Policy.Quality)
		if err != nil || allocation != state.Allocation[receiverIndex] {
			return errCheckpoint
		}

		if state.Completed[receiverIndex] {
			production := state.Production[receiverIndex]
			if production.ReceiverID != receiver.ID || production.Count != allocation.Samples {
				return errCheckpoint
			}

			if _, err = EstimateStatistics(production); err != nil {
				return err
			}
		}
	}

	return nil
}

// BakeAdaptive runs the hardware worker with independently allocated production
// counts and resumable frozen state. It never mixes the pilot into production.
func (worker *GPUWorker) BakeAdaptive(
	ctx context.Context,
	scene *PreparedScene,
	receivers []Receiver,
	options Options,
	policy AdaptivePolicy,
	resume *AdaptiveState,
	save func(AdaptiveState) error,
) (Result, *AdaptiveState, error) {
	if worker == nil {
		return Result{}, nil, ErrInvalid
	}

	return BakeAdaptive(ctx, worker, scene, receivers, options, policy, worker.Device().Adapter, resume, save)
}

func (state AdaptiveState) sameIdentity(expected AdaptiveState, count int) bool {
	return state.Schema == AdaptiveSchema && state.Identity == expected.Identity && state.SceneHash == expected.SceneHash &&
		state.ReceiverHash == expected.ReceiverHash &&
		state.Backend == expected.Backend &&
		state.Options == expected.Options &&
		state.Policy == expected.Policy &&
		state.PilotSeed == expected.PilotSeed &&
		state.ProductionSeed == expected.ProductionSeed &&
		len(state.Pilot) == count &&
		len(state.Allocation) == count &&
		len(state.Completed) == count &&
		len(state.Production) == count
}
