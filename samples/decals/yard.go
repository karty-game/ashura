package main

import (
	"github.com/karty-game/ashura/pkg/decal"
	"github.com/karty-game/ashura/pkg/engine"
	"github.com/karty-game/ashura/pkg/material"
	"github.com/karty-game/ashura/pkg/mesh"
	"github.com/karty-game/ashura/pkg/texture"
	"github.com/karty-game/ashura/samples/decals/generated/assets"
	"github.com/karty-game/ashura/samples/decals/internal/room"
)

func buildYard(app *engine.App, world *engine.World) error {
	concrete, err := assets.YardConcreteAlbedo().Load(app)
	if err != nil {
		return err
	}

	railcar, err := assets.YardRailcarAlbedo().Load(app)
	if err != nil {
		return err
	}

	asphalt, err := assets.YardAsphaltAlbedo().Load(app)
	if err != nil {
		return err
	}

	floorGeometry, floorPose := room.Floor()
	floor := material.DefaultSource()
	floor.AlbedoTexture = asphalt
	world.Add(floorGeometry, floorPose, floor, decal.Receiver{Layers: room.FloorLayer})

	unit := mesh.NewBox("switchyard structure", mesh.BoxOptions{})
	finishes := yardFinishes(concrete, railcar)

	for _, surface := range room.Surfaces() {
		transform := surfaceTransform(surface)
		shape := unit.CreateInstance("switchyard structure")

		finish := finishes[surface.Finish]
		if surface.Layers != 0 {
			world.Add(shape, transform, finish, decal.Receiver{Layers: surface.Layers})
		} else {
			world.Add(shape, transform, finish)
		}
	}

	return nil
}

func surfaceTransform(geometry room.Surface) engine.Transform {
	const half = .5

	lo, hi := geometry.Bounds.Min, geometry.Bounds.Max

	return engine.Transform{
		Position: engine.Vector3{X: (hi.X + lo.X) * half, Y: (hi.Y + lo.Y) * half, Z: (hi.Z + lo.Z) * half},
		Scale:    engine.Vector3{X: hi.X - lo.X, Y: hi.Y - lo.Y, Z: hi.Z - lo.Z},
	}
}

//nolint:mnd // Material tints are authored in linear space.
func yardFinishes(concrete, railcar *texture.Texture) [room.Lamp + 1]material.Material {
	makeFinish := func(color engine.Color, roughness float32) material.Material {
		result := material.DefaultStandard()
		result.Color, result.Roughness = color, roughness

		return result
	}
	colors := [room.Lamp + 1]material.Material{
		room.Concrete: makeFinish(engine.Color{R: .84, G: .83, B: .80, A: 1}, .91),
		room.Charcoal: makeFinish(engine.Color{R: .055, G: .074, B: .091, A: 1}, .84),
		room.Steel:    makeFinish(engine.Color{R: .20, G: .24, B: .27, A: 1}, .67),
		room.Rust:     makeFinish(engine.Color{R: .41, G: .16, B: .091, A: 1}, .87),
		room.Teal:     makeFinish(engine.Color{R: .18, G: .56, B: .56, A: 1}, .74),
		room.Amber:    makeFinish(engine.Color{R: .78, G: .44, B: .10, A: 1}, .69),
		room.Lamp:     makeFinish(engine.Color{R: 1, G: .81, B: .37, A: 1}, .54),
	}
	colors[room.Concrete].AlbedoTexture = concrete
	colors[room.Concrete].UVScale = engine.Vector2{X: 2, Y: 1}
	colors[room.Teal].AlbedoTexture = railcar

	return colors
}
