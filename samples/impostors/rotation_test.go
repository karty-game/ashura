package main

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/karty-game/ashura/pkg/camera"
	"github.com/karty-game/ashura/pkg/engine"
	"github.com/karty-game/ashura/pkg/impostor"
	"github.com/karty-game/ashura/pkg/light"
)

func TestSharedPoseKeepsModelAndImpostorCentered(t *testing.T) {
	t.Parallel()

	comparison := comparisonFixture()
	comparison.state.rotation = engine.Vector3{Y: math.Pi / 2}
	comparison.applyPose()

	for _, transform := range comparison.modelTransforms {
		assertVector(t, transform.Position, engine.Vector3{X: -2 - pairOffset, Z: 4})
		assertVector(t, transform.Rotation, comparison.impostorTransform.Rotation)
		assertVector(t, transform.Scale, engine.Vector3One())
	}

	assertVector(t, comparison.impostorTransform.Position, engine.Vector3{X: -2 + pairOffset, Z: 4})
	assertVector(t, comparison.impostorTransform.Scale, engine.Vector3One())
	// Rx maps (1,2,3) to (1,-3,2), Ry to (2,-3,-1), then Rz to (3,2,-1).
	assertVector(
		t,
		rotateCenter(engine.Vector3{X: 1, Y: 2, Z: 3}, engine.Vector3{X: math.Pi / 2, Y: math.Pi / 2, Z: math.Pi / 2}),
		engine.Vector3{X: 3, Y: 2, Z: -1},
	)
}

func TestMotionModesResetAndEventOnlyStatus(t *testing.T) {
	t.Parallel()

	comparison := comparisonFixture()
	comparison.view.Rotation.X = -1.2
	notifications := 0

	comparison.notify = func(int, rotationMode) { notifications++ }
	if err := comparison.update(engine.Input{}, time.Second); err != nil {
		t.Fatal(err)
	}

	if comparison.view.Rotation.X != -1.2 || notifications != 0 {
		t.Fatal("ordinary frames restricted orbit elevation or rewrote status")
	}

	for _, expected := range []rotationMode{rotationYaw, rotationPitch, rotationRoll, rotationPaused} {
		comparison.requestRotation()

		if err := comparison.update(engine.Input{}, 0); err != nil {
			t.Fatal(err)
		}

		if comparison.state.mode != expected || comparison.state.mode.label() == "" {
			t.Fatalf("mode=%v,want%v", comparison.state.mode, expected)
		}

		before := notifications

		if err := comparison.update(engine.Input{}, time.Second); err != nil {
			t.Fatal(err)
		}

		if notifications != before {
			t.Fatal("continuous motion rewrote event-only status")
		}

		for _, transform := range comparison.modelTransforms {
			assertVector(t, transform.Rotation, comparison.impostorTransform.Rotation)
		}
	}

	comparison.requestLight()
	comparison.requestReset()

	if err := comparison.update(engine.Input{}, 0); err != nil {
		t.Fatal(err)
	}

	if comparison.state.mode != rotationPaused || comparison.state.lightIndex != 1 || notifications != 5 {
		t.Fatalf("reset/light status incorrect: %+v notifications%d", comparison.state, notifications)
	}

	assertVector(t, comparison.state.rotation, engine.Vector3{})
	assertVector(t, comparison.modelTransforms[0].Position, engine.Vector3{X: -pairOffset})
	assertVector(t, comparison.impostorTransform.Position, engine.Vector3{X: pairOffset})
	assertVector(t, comparison.view.Rotation, engine.Vector3{X: initialPitch})
}

//nolint:paralleltest // Go 1.27 forbids AllocsPerRun inside parallel tests.
func TestContinuousComparisonMotionDoesNotAllocate(t *testing.T) {
	// AllocsPerRun must run outside parallel tests on the pinned Go compiler.
	comparison := comparisonFixture()
	comparison.state.mode = rotationYaw

	var updateErr error

	allocations := testing.AllocsPerRun(1000, func() { updateErr = comparison.update(engine.Input{}, time.Second/60) })
	if allocations != 0 || updateErr != nil {
		t.Fatalf("continuous pose updates allocated %g times per frame, error %v", allocations, updateErr)
	}
}

func comparisonFixture() *comparisonScene {
	first, second, card := engine.DefaultTransform(), engine.DefaultTransform(), engine.DefaultTransform()

	return &comparisonScene{
		modelTransforms: []*engine.Transform{&first, &second}, impostorTransform: &card, center: engine.Vector3{X: 1, Y: 2, Z: 3},
		orbit: &camera.OrbitControls{Target: engine.Vector3{Y: viewTargetHeight}, Distance: minimumViewDistance},
		view:  &engine.Transform{}, sun: &light.Directional{}, distance: minimumViewDistance,
	}
}

func assertVector(t *testing.T, actual, expected engine.Vector3) {
	t.Helper()

	const tolerance = 1e-5
	if math.Abs(float64(actual.X-expected.X)) > tolerance || math.Abs(float64(actual.Y-expected.Y)) > tolerance ||
		math.Abs(float64(actual.Z-expected.Z)) > tolerance {
		t.Fatalf("vector%+v,want%+v", actual, expected)
	}
}

func TestQualityFailureReachesUpdateCaller(t *testing.T) {
	t.Parallel()

	comparison := comparisonFixture()

	comparison.qualityAsset = &impostor.Impostor{}
	if err := comparison.requestQuality(impostor.DefaultQuality()); err != nil {
		t.Fatal(err)
	}

	if err := comparison.update(engine.Input{}, time.Second); !errors.Is(err, impostor.ErrClosed) {
		t.Fatalf("unavailable quality resource must reach the update caller: %v", err)
	}
}

func TestSelectedModelCenterSurvivesUpdatesAndReset(t *testing.T) {
	t.Parallel()

	comparison := comparisonFixture()
	for _, height := range []float32{.4, 2.6} {
		// Model switches preserve a close orbit while replacing its target height.
		comparison.orbit.Distance = minimumViewDistance * minimumZoomFactor
		comparison.view.Rotation = engine.Vector3{X: -.4, Y: .3}
		comparison.attach(comparison.modelTransforms, comparison.impostorTransform, engine.Vector3{Y: height})
		position, rotation, distance := comparison.view.Position, comparison.view.Rotation, comparison.orbit.Distance

		if err := comparison.update(engine.Input{}, time.Second/60); err != nil {
			t.Fatal(err)
		}

		assertVector(t, comparison.orbit.Target, engine.Vector3{Y: height})
		assertVector(t, comparison.view.Position, position)
		assertVector(t, comparison.view.Rotation, rotation)

		if comparison.orbit.Distance != distance {
			t.Fatal("model selection changed the close orbit distance")
		}

		comparison.requestReset()

		if err := comparison.update(engine.Input{}, 0); err != nil {
			t.Fatal(err)
		}

		assertVector(t, comparison.orbit.Target, engine.Vector3{Y: height})
		assertVector(t, comparison.view.Position, engine.Vector3{
			Y: height - float32(math.Sin(initialPitch))*minimumViewDistance,
			Z: float32(math.Cos(initialPitch)) * minimumViewDistance,
		})

		if comparison.orbit.Distance != minimumViewDistance {
			t.Fatal("reset did not restore the initial orbit distance")
		}
	}
}
