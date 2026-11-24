package main

import (
	"math"
	"time"

	"github.com/karty-game/ashura/pkg/camera"
	"github.com/karty-game/ashura/pkg/engine"
)

const (
	courtyardView = iota
	galleryView
	flightView
	ceilingView
	viewCount
)

const (
	viewYaw                         = math.Pi / 2
	courtyardHeight, courtyardRange = 3, 11
	courtyardPitch                  = .05
	galleryHeight, galleryDepth     = 4.3, -4.9
	galleryRange                    = 6
	galleryPitch                    = -.35
	ceilingHeight, ceilingPitch     = 9, .4
	flightStartX, flightStartY      = 10, 1.8
)

func createWorld(app *engine.App) (*engine.World, func(int), error) {
	world := engine.NewWorld()
	projection, orbit := camera.Orbit()
	orbit.MinDistance, orbit.MaxDistance = .5, 60
	view := world.Add(engine.DefaultTransform(), projection, orbit)
	flightProjection, flight := camera.FirstPerson()
	flight.MoveSpeed = 4
	flightEntity := world.Add(flightPose(), flightProjection, flight)
	controls, _ := engine.Get[camera.OrbitControls](world, view)
	current, pending := courtyardView, courtyardView
	previous := [viewCount + 1]bool{}
	selectView := func(index int) { pending = index }

	app.OnUpdateError(func(time.Duration) error {
		keys := [...]engine.Key{engine.Key1, engine.Key2, engine.Key3, engine.KeyB, engine.KeyR}
		for index, key := range keys {
			held := app.Input().KeyDown(key)
			if held && !previous[index] {
				pending = index
				if index == viewCount {
					pending = current
				}
			}

			previous[index] = held
		}

		if pending >= 0 && pending < viewCount {
			current = pending
			if err := applyView(world, view, flightEntity, controls, current); err != nil {
				return err
			}

			pending = -1
		}

		return nil
	})

	if err := applyView(world, view, flightEntity, controls, courtyardView); err != nil {
		return nil, nil, err
	}

	return world, selectView, nil
}

func applyView(world *engine.World, orbit, flight engine.Entity, controls *camera.OrbitControls, index int) error {
	if index == flightView {
		*world.Transform(flight) = flightPose()

		return world.SetActiveCamera(flight)
	}

	controls.Target, controls.Distance = engine.Vector3{X: -1, Y: courtyardHeight}, courtyardRange

	rotation := engine.Vector3{X: courtyardPitch, Y: viewYaw}
	if index == galleryView {
		controls.Target, controls.Distance = engine.Vector3{X: -2, Y: galleryHeight, Z: galleryDepth}, galleryRange
		rotation.X = galleryPitch
	}

	if index == ceilingView {
		controls.Target, controls.Distance = engine.Vector3{X: -2, Y: ceilingHeight, Z: galleryDepth}, galleryRange
		rotation.X = ceilingPitch
	}

	*world.Transform(orbit) = engine.Transform{Rotation: rotation, Scale: engine.Vector3One()}

	return world.SetActiveCamera(orbit)
}

func flightPose() engine.Transform {
	return engine.Transform{
		Position: engine.Vector3{X: flightStartX, Y: flightStartY},
		Rotation: engine.Vector3{Y: viewYaw},
		Scale:    engine.Vector3One(),
	}
}
