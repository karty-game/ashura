package main

import (
	"math"

	"github.com/karty-game/ashura/pkg/engine"
	"github.com/karty-game/ashura/pkg/light"
	"github.com/karty-game/ashura/pkg/mesh"
)

const (
	lowerAccentCount = extraLightCount - 24
	groundMoteRange  = 1.95
	towerMoteRange   = 2.70
)

//nolint:mnd // The three colors and crystal dimensions are authored lighting.
func (rig *lightRig) addAccents(world *engine.World) {
	palette := [...]engine.Color{
		{R: .12, G: .83, B: 1, A: 1},
		{R: .56, G: .27, B: 1, A: 1},
		{R: 1, G: .47, B: .13, A: 1},
	}
	crystal := mesh.NewBox("lantern mote", mesh.BoxOptions{Width: .092, Height: .125, Depth: .092})

	for index := range rig.accents {
		color := palette[index%len(palette)]
		point := light.DefaultPoint("lantern mote")
		point.Color, point.Range = color, groundMoteRange
		rig.accents[index], _ = engine.Get[light.Point](world, world.Add(point))

		finish := surface(engine.ColorWhite())
		finish.AmbientColors = [3]engine.Color{color, color, color}
		start := at(0, -1, 0)
		start.Rotation.X, start.Rotation.Z = .55, math.Pi/4
		rig.markers[index] = world.Transform(world.Add(crystal, start, finish))
	}
}

//nolint:mnd // Animation speed, intensity and range describe the authored mote fields.
func (rig *lightRig) updateAccents() {
	count := extraLightCount
	if rig.mode == 1 {
		count = sparseLightCount - heroLightCount
	}

	for index, point := range rig.accents {
		if rig.bakedOnly || index >= count {
			point.Intensity = 0
			rig.markers[index].Position.Y = -1

			continue
		}

		placement := index * extraLightCount / count
		phase := rig.elapsed*.92 + float64(placement)*.56
		point.Position = accentPosition(placement, phase, rig.mode == 2)

		point.Range, point.Intensity = groundMoteRange, .59+float32(math.Sin(phase))*.21
		if rig.mode == 2 {
			point.Range, point.Intensity = 8, .024
		} else if placement >= lowerAccentCount {
			point.Range, point.Intensity = towerMoteRange, .9+float32(math.Sin(phase))*.19
		}

		rig.markers[index].Position = point.Position
	}
}

//nolint:mnd // The lanes and height waves clear the columns and provide three contrasting light bands.
func accentPosition(index int, phase float64, overlap bool) engine.Vector3 {
	lanes := [...]float32{-9.7, -7.5, -3.85, -1.52, 1.52, 3.85, 7.5, 9.7}
	row := float32(index / len(lanes))
	sway := float32(math.Sin(phase))
	lift := float32(math.Cos(phase))

	if overlap {
		radius := .65 + row*.06

		return engine.Vector3{X: sway * radius, Y: 1.9 + lift*.9, Z: 1.65 + lift*radius}
	}

	if index >= lowerAccentCount {
		arches := [...]float32{-10.45, -6.1, -2.25, 2.25, 6.1, 10.45}
		index -= lowerAccentCount

		return engine.Vector3{
			X: arches[index%len(arches)] + sway*.46,
			Y: 3.16 + float32(index/len(arches))*1.26 + lift*.24,
			Z: -4.27 + sway*.22,
		}
	}

	return engine.Vector3{
		X: lanes[index%len(lanes)] + sway*.38,
		Y: .92 + lift*.38 + float32(index%3)*.62,
		Z: -4.85 + row*.53 + sway*.26,
	}
}
