package main

import (
	"github.com/karty-game/ashura/pkg/ui"
	"github.com/karty-game/ashura/samples/ui/generated/assets"
)

type artwork struct {
	game   gameArt
	panels panelArt
}

type gameArt struct {
	coin, gem, potion, shield *ui.ImageAsset
}

type panelArt struct {
	gold, steel, landscape *ui.ImageAsset
}

// loadArtwork admits each prepared catalog page once for this application run.
// The button and panel examples share its named regions; no source codec runs.
func loadArtwork() (artwork, error) {
	images, err := assets.Load()
	if err != nil {
		return artwork{}, err
	}

	return artwork{
		game:   gameArt{coin: images.UiCoin, gem: images.UiGem, potion: images.UiPotion, shield: images.UiShield},
		panels: panelArt{gold: images.UiGoldFrame, steel: images.UiSteelFrame, landscape: images.UiLandscape},
	}, nil
}
