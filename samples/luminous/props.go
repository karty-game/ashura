package main

import (
	"math"

	"github.com/karty-game/ashura/pkg/engine"
	"github.com/karty-game/ashura/pkg/model"
	"github.com/karty-game/ashura/samples/luminous/generated/assets"
	"github.com/karty-game/ashura/samples/luminous/generated/assets/models"
)

type propGroup struct {
	asset model.Asset
	roots []engine.Transform
}

func addCastle(app *engine.App, world *engine.World) (*model.Instance, error) {
	palette, err := assets.CastleAlbedo().Load(app)
	if err != nil {
		return nil, err
	}

	finish := surface(engine.ColorWhite())
	finish.AlbedoTexture = palette

	for _, group := range castleProps() {
		loaded, loadErr := group.asset.Load(app)
		if loadErr != nil {
			return nil, loadErr
		}

		for _, root := range group.roots {
			if _, placeErr := loaded.InstantiateWithMaterial(world, root, finish); placeErr != nil {
				return nil, placeErr
			}
		}
	}

	ballista, err := models.AssetsSiegeBallista().Load(app)
	if err != nil {
		return nil, err
	}

	return ballista.InstantiateWithMaterial(world, ballistaPose(0), finish)
}

//nolint:mnd // Scene scale, position and movement are authored for the ballista.
func ballistaPose(elapsed float64) engine.Transform {
	x := float32(math.Sin(elapsed*.63) * 2.25)
	yaw := -.85 + float32(math.Sin(elapsed*.49))*.6

	return propAt(x, .02, 2.85, 2.15, 2.15, 2.15, yaw)
}

func castleProps() []propGroup {
	return []propGroup{
		{models.AssetsTowerSquareArch(), []engine.Transform{propAt(0, 0, -6.9, 5.7, 5.4, 3.6, 0)}},
		{models.AssetsWallHalf(), []engine.Transform{
			propAt(-5.85, 0, -7.25, 6.2, 3.7, 1.6, 0),
			propAt(5.85, 0, -7.25, 6.2, 3.7, 1.6, 0),
		}},
		{models.AssetsTowerSquare(), []engine.Transform{
			propAt(-10.25, 0, -7.25, 3.5, 4.6, 3.5, 0),
			propAt(10.25, 0, -7.25, 3.5, 4.6, 3.5, 0),
		}},
		{models.AssetsTowerSquareTopRoof(), []engine.Transform{
			propAt(-10.25, 5.7, -7.25, 4, 2.95, 4, 0),
			propAt(10.25, 5.7, -7.25, 4, 2.95, 4, 0),
		}},
		{models.AssetsFlagBannerLong(), []engine.Transform{
			propAt(-3.9, 1.1, -6.39, 1.6, 1.6, 1.6, math.Pi/2),
			propAt(3.9, 1.1, -6.39, 1.6, 1.6, 1.6, math.Pi/2),
			propAt(-10.25, 2.3, -5.44, 1.7, 1.7, 1.7, math.Pi/2),
			propAt(10.25, 2.3, -5.44, 1.7, 1.7, 1.7, math.Pi/2),
		}},
		{models.AssetsTreeSmall(), []engine.Transform{
			propAt(-11.9, 0, .2, 3, 3, 3, .5),
			propAt(11.8, 0, -1.75, 2.85, 3.55, 2.85, -.45),
			propAt(-10.7, 0, 7.3, 2.15, 2.45, 2.15, -.8),
			propAt(11.6, 0, 5, 2.8, 2.8, 2.8, .9),
		}},
	}
}

func propAt(x, y, z, sx, sy, sz, yaw float32) engine.Transform {
	root := at(x, y, z)
	root.Scale = engine.Vector3{X: sx, Y: sy, Z: sz}
	root.Rotation.Y = yaw

	return root
}
