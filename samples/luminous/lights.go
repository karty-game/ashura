package main

import (
	"math"
	"time"

	"github.com/karty-game/ashura/pkg/engine"
	"github.com/karty-game/ashura/pkg/light"
	"github.com/karty-game/ashura/pkg/model"
)

const (
	lightCount       = 240
	sparseLightCount = 96
	heroLightCount   = 3
	extraLightCount  = lightCount - heroLightCount
)

type lightRig struct {
	point             *light.Point
	spots             [2]*light.Spot
	accents           [extraLightCount]*light.Point
	markers           [extraLightCount]*engine.Transform
	ballista          *model.Instance
	mode              int
	bakedOnly, paused bool
	elapsed           float64
}

//nolint:mnd // Shadow priorities, colors and cones are authored for the three hero fixtures.
func addLights(world *engine.World) *lightRig {
	rig := &lightRig{}
	point := light.DefaultPoint("amber point")
	point.Range = 7
	point.Color = engine.Color{R: 1, G: .42, B: .15, A: 1}
	point.CastShadows = true
	rig.point, _ = engine.Get[light.Point](world, world.Add(point))

	blue := light.DefaultSpot("near ice spotlight")
	blue.Position, blue.Direction = engine.Vector3{X: -3.5, Y: 5.6, Z: 6.7}, engine.Vector3{X: .28, Y: -.82, Z: -.57}
	blue.Color, blue.Range, blue.CastShadows = engine.Color{R: .35, G: .69, B: 1, A: 1}, 11, true
	blue.InnerAngle, blue.OuterAngle = .30, .50
	pink := light.DefaultSpot("rear rose spotlight")
	pink.Position, pink.Direction = engine.Vector3{X: 6.4, Y: 6.2, Z: -5.2}, engine.Vector3{X: -.36, Y: -.83, Z: .36}

	pink.Color, pink.Range, pink.CastShadows = engine.Color{R: 1, G: .28, B: .56, A: 1}, 12, true
	pink.InnerAngle, pink.OuterAngle = .35, .51

	rig.spots[0], _ = engine.Get[light.Spot](world, world.Add(blue))
	rig.spots[1], _ = engine.Get[light.Spot](world, world.Add(pink))

	rig.addAccents(world)

	return rig
}

func (rig *lightRig) bind(app *engine.App) {
	keys := [...]engine.Key{engine.Key1, engine.Key2, engine.Key3, engine.KeyB, engine.KeyH}

	var held [len(keys)]bool

	app.OnUpdateError(func(delta time.Duration) error {
		input := app.Input()
		changed := false

		for index, key := range keys {
			down := input.KeyDown(key)
			if down && !held[index] {
				rig.control(app, key)

				changed = true
			}

			held[index] = down
		}

		if !rig.paused {
			rig.elapsed += delta.Seconds()
		}

		if !rig.paused || changed {
			return rig.update()
		}

		return nil
	})
}

func (rig *lightRig) control(app *engine.App, key engine.Key) {
	switch key { //nolint:exhaustive // Only Luminous controls are routed to this handler.
	case engine.Key1:
		app.SetLightCulling(true)
	case engine.Key2:
		app.SetLightCulling(false)
	case engine.Key3:
		const lightLayouts = 3

		rig.mode = (rig.mode + 1) % lightLayouts
	case engine.KeyB:
		rig.bakedOnly = !rig.bakedOnly
	case engine.KeyH:
		rig.paused = !rig.paused
	}
}

//nolint:mnd // Animation paths, timesteps and accent positions are authored values.
func (rig *lightRig) update() error {
	elapsed := rig.elapsed

	if rig.ballista != nil {
		if err := rig.ballista.SetTransform(ballistaPose(elapsed)); err != nil {
			return err
		}
	}
	// Keep the sweep in front of the nearest pillars.
	rig.point.Position = engine.Vector3{X: 4.4 + float32(math.Sin(elapsed*.58)*1.35), Y: 2.5, Z: 5.9}
	rig.spots[0].Direction.X = .28 + float32(math.Sin(elapsed*.37)*.18)

	mainIntensity := float32(1)
	if rig.bakedOnly {
		mainIntensity = 0
	}

	rig.point.Intensity = 1.7 * mainIntensity
	rig.spots[0].Intensity, rig.spots[1].Intensity = 2.65*mainIntensity, 2.3*mainIntensity

	rig.updateAccents()

	return nil
}
