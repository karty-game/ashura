package main

import (
	"math"
	"time"

	"github.com/karty-game/karty-webgpu/pkg/camera"
	"github.com/karty-game/karty-webgpu/pkg/engine"
	"github.com/karty-game/karty-webgpu/pkg/impostor"
	"github.com/karty-game/karty-webgpu/pkg/light"
	"github.com/karty-game/karty-webgpu/pkg/material"
	"github.com/karty-game/karty-webgpu/pkg/mesh"
	"github.com/karty-game/karty-webgpu/samples/internal/sample"
)

const (
	pairOffset                     = 3.2
	viewTargetHeight               = 1.5
	initialPitch                   = -.2
	minimumViewDistance            = 20
	groundHeight                   = -.15
	keyLight            engine.Key = 'L'
	keyRotate           engine.Key = 'T'
	minimumZoomFactor              = .65
	maximumZoomFactor              = 25
	skyIntensity                   = .55
	sunIntensity                   = 1.1
	groundWidth                    = 20
	groundDepth                    = 18
	groundRed                      = .07
	groundGreen                    = .085
	groundBlue                     = .095
	rotationSpeed                  = .55
)

type comparisonControls struct {
	lightPending, resetPending, rotatePending    bool
	previousLight, previousReset, previousRotate bool
	lightIndex                                   int
	mode                                         rotationMode
	rotation                                     engine.Vector3
	poseDirty                                    bool
}

type comparisonScene struct {
	qualityAsset      *impostor.Impostor
	qualityPending    bool
	requestedQuality  impostor.Quality
	state             comparisonControls
	sun               *light.Directional
	orbit             *camera.OrbitControls
	view              *engine.Transform
	distance          float32
	center            engine.Vector3
	modelTransforms   []*engine.Transform
	impostorTransform *engine.Transform
	notify            func(int, rotationMode)
	prepare           func() error
}

func createScene(app *engine.App) (*engine.World, *comparisonScene, error) {
	world := engine.NewWorld()
	projection, controls := camera.Orbit()
	distance := float32(minimumViewDistance)
	controls.Target, controls.Distance = engine.Vector3{Y: viewTargetHeight}, distance
	controls.MinDistance, controls.MaxDistance = minimumViewDistance*minimumZoomFactor, distance*maximumZoomFactor

	view := world.Add(engine.Transform{Rotation: engine.Vector3{X: initialPitch}, Scale: engine.Vector3One()}, projection, controls)
	if err := world.SetActiveCamera(view); err != nil {
		return nil, nil, err
	}

	liveControls, _ := engine.Get[camera.OrbitControls](world, view)
	sky := light.DefaultHemisphere("sky")
	sky.Intensity = skyIntensity
	world.Add(sky)

	source := light.DefaultDirectional("sun")
	source.Direction, source.Intensity = comparisonLightDirections()[0], sunIntensity
	sunEntity := world.Add(source)
	sun, _ := engine.Get[light.Directional](world, sunEntity)
	ground := material.DefaultStandard()
	ground.Color, ground.Roughness = engine.Color{R: groundRed, G: groundGreen, B: groundBlue, A: 1}, 1
	world.Add(mesh.NewPlane("ground", mesh.PlaneOptions{Width: groundWidth, Height: groundDepth}),
		engine.Transform{Position: engine.Vector3{Y: groundHeight}, Scale: engine.Vector3One()},
		ground,
	)
	comparison := &comparisonScene{sun: sun, orbit: liveControls, view: world.Transform(view), distance: distance}

	sample.FitCamera(app, world, view)
	app.OnUpdateError(func(delta time.Duration) error { return comparison.update(app.Input(), delta) })

	return world, comparison, nil
}

func (comparison *comparisonScene) requestLight()    { comparison.state.lightPending = true }
func (comparison *comparisonScene) requestReset()    { comparison.state.resetPending = true }
func (comparison *comparisonScene) requestRotation() { comparison.state.rotatePending = true }

func (comparison *comparisonScene) attach(modelTransforms []*engine.Transform, impostorTransform *engine.Transform, center engine.Vector3) {
	comparison.modelTransforms, comparison.impostorTransform, comparison.center = modelTransforms, impostorTransform, center
	comparison.orbit.Target.Y = center.Y
	positionCamera(comparison.orbit, comparison.view)
	comparison.applyPose()
}

func (comparison *comparisonScene) requestQuality(settings impostor.Quality) error {
	if err := settings.Validate(); err != nil {
		return err
	}

	comparison.requestedQuality, comparison.qualityPending = settings, true

	return nil
}

func (comparison *comparisonScene) update(input engine.Input, delta time.Duration) error {
	if comparison.prepare != nil {
		if err := comparison.prepare(); err != nil {
			return err
		}
	}

	if comparison.qualityPending {
		comparison.qualityPending = false
		if err := comparison.qualityAsset.SetQuality(comparison.requestedQuality); err != nil {
			return err
		}
	}

	state := &comparison.state
	heldLight, heldReset, heldRotate := input.KeyDown(keyLight), input.KeyDown(engine.KeyR), input.KeyDown(keyRotate)
	state.lightPending = state.lightPending || heldLight && !state.previousLight
	state.resetPending = state.resetPending || heldReset && !state.previousReset
	state.rotatePending = state.rotatePending || heldRotate && !state.previousRotate
	state.previousLight, state.previousReset, state.previousRotate = heldLight, heldReset, heldRotate
	changed := comparison.consumeRequests()

	if state.mode != rotationPaused && delta > 0 {
		state.rotation = advanceRotation(state.rotation, state.mode, float32(delta.Seconds()*rotationSpeed))
		state.poseDirty = true
	}

	if state.poseDirty {
		comparison.applyPose()

		state.poseDirty = false
	}

	// Keep the pair centered; the engine's existing orbit controls retain their
	// normal near-pole bounds, with no capture-elevation restriction here.
	target := engine.Vector3{Y: comparison.center.Y}
	if comparison.orbit.Target != target {
		comparison.orbit.Target = target
		positionCamera(comparison.orbit, comparison.view)
	}

	if changed && comparison.notify != nil {
		comparison.notify(state.lightIndex, state.mode)
	}

	return nil
}

func (comparison *comparisonScene) consumeRequests() bool {
	state := &comparison.state

	changed := state.lightPending || state.resetPending || state.rotatePending
	if state.lightPending {
		directions := comparisonLightDirections()
		state.lightIndex = (state.lightIndex + 1) % len(directions)
		comparison.sun.Direction = directions[state.lightIndex]
		state.lightPending = false
	}

	if state.rotatePending {
		state.mode = (state.mode + 1) % rotationModeCount
		state.rotatePending = false
	}

	if state.resetPending {
		comparison.orbit.Distance = comparison.distance
		comparison.view.Rotation = engine.Vector3{X: initialPitch}
		comparison.orbit.Target = engine.Vector3{Y: comparison.center.Y}
		positionCamera(comparison.orbit, comparison.view)

		state.rotation, state.mode, state.poseDirty = engine.Vector3{}, rotationPaused, true
		state.resetPending = false
	}

	return changed
}

//nolint:mnd // Three authored sun directions remain explicit for the relighting comparison.
func comparisonLightDirections() [3]engine.Vector3 {
	return [3]engine.Vector3{{X: -.6, Y: -1, Z: -.4}, {X: .8, Y: -.5, Z: .3}, {X: -.2, Y: -.3, Z: 1}}
}

func positionCamera(controls *camera.OrbitControls, transform *engine.Transform) {
	pitch, yaw := float64(transform.Rotation.X), float64(transform.Rotation.Y)
	cosine := math.Cos(pitch)
	transform.Position = engine.Vector3{
		X: controls.Target.X + float32(math.Sin(yaw)*cosine)*controls.Distance,
		Y: controls.Target.Y - float32(math.Sin(pitch))*controls.Distance,
		Z: controls.Target.Z + float32(math.Cos(yaw)*cosine)*controls.Distance,
	}
}
