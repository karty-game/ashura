package main

import (
	"github.com/karty-game/ashura/pkg/engine"
	"github.com/karty-game/ashura/pkg/model"
	"github.com/karty-game/ashura/pkg/texture"
	"github.com/karty-game/ashura/samples/luminous/generated/assets"
	"github.com/karty-game/ashura/samples/luminous/internal/courtyard"
)

type floorLayout []byte

func (layout floorLayout) ModelDigest() [32]byte { return [32]byte(layout[:32]) }
func (floorLayout) PartCount() int               { return 1 }
func (layout floorLayout) PartUV(int) []byte     { return layout[32:] }

func addFloor(app *engine.App, world *engine.World) error {
	floorModel, floorUV, floorLight := assets.BakedFloor()

	floor, err := model.Prepared("Luminous floor", floorModel).LoadLightmapped(app, floorLayout(floorUV))
	if err != nil {
		return err
	}

	irradiance, err := app.AdoptTexture(texture.Data{
		Width: courtyard.BakeResolution, Height: courtyard.BakeResolution, ColorSpace: texture.Linear,
		Levels: [][]byte{floorLight},
	})
	if err != nil {
		return err
	}

	albedo, err := assets.CobblestoneAlbedo().Load(app)
	if err != nil {
		return err
	}

	relief, err := assets.CobblestoneSSBump().Load(app)
	if err != nil {
		return err
	}

	const repeatsX, repeatsZ, reliefStrength = 12, 9, .55

	floorSurface := surface(engine.Color{R: courtyard.FloorRed, G: courtyard.FloorGreen, B: courtyard.FloorBlue, A: 1})
	floorSurface.AlbedoTexture, floorSurface.SSBumpTexture = albedo, relief
	floorSurface.UVScale = engine.Vector2{X: repeatsX, Y: repeatsZ}
	floorSurface.SSBumpStrength = reliefStrength
	// A flat receiver weights each RNM basis equally. Sharing the scalar
	// bake preserves its irradiance without allocating three textures.
	floorSurface.RNMLightmap = &texture.RNMLightmap{
		Textures: [3]*texture.Texture{irradiance, irradiance, irradiance},
		Scale:    1,
		Encoding: texture.RNMLinear,
	}
	_, err = floor.InstantiateWithMaterial(world, engine.DefaultTransform(), floorSurface)

	return err
}
