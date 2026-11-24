package main

import (
	"errors"

	"github.com/karty-game/ashura/pkg/engine"
	"github.com/karty-game/ashura/pkg/model"
	"github.com/karty-game/ashura/pkg/scene/resource"
	"github.com/karty-game/ashura/pkg/texture"
	impostors "github.com/karty-game/ashura/samples/impostors/generated/impostors/bc"
	mobile "github.com/karty-game/ashura/samples/impostors/generated/impostors/mobile"
	"github.com/karty-game/ashura/samples/impostors/generated/models"
)

var errImpostorCompression = errors.New("impostors requires BC or ASTC texture compression")

type comparisonExample struct {
	id, label string
	model     model.Asset
	card      resource.Asset
}

func comparisonExamplesForDevice(app *engine.App) ([3]comparisonExample, error) {
	examples := comparisonExamples()
	if app.SupportsTextureFormat(texture.BC1) {
		return examples, nil
	}

	if !app.SupportsTextureFormat(texture.ASTC6x6) {
		return [3]comparisonExample{}, errImpostorCompression
	}

	for index, card := range [3]resource.Asset{mobile.AssetsBoulder(), mobile.AssetsStool(), mobile.AssetsVase()} {
		examples[index].card = card
	}

	return examples, nil
}

func comparisonExamples() [3]comparisonExample {
	return [3]comparisonExample{
		{id: "boulder", label: "Boulder", model: models.AssetsBoulder(), card: impostors.AssetsBoulder()},
		{id: "stool", label: "Wooden stool", model: models.AssetsStool(), card: impostors.AssetsStool()},
		{id: "vase", label: "Ceramic vase", model: models.AssetsVase(), card: impostors.AssetsVase()},
	}
}

func selectExample(examples [3]comparisonExample, id string) comparisonExample {
	for _, example := range examples {
		if example.id == id {
			return example
		}
	}

	return examples[0]
}
