package main

import (
	"github.com/karty-game/karty-webgpu/pkg/impostor"
	"github.com/karty-game/karty-webgpu/pkg/model"
	"github.com/karty-game/karty-webgpu/samples/impostors/generated/impostors"
	"github.com/karty-game/karty-webgpu/samples/impostors/generated/models"
)

type comparisonExample struct {
	id, label string
	model     model.Asset
	card      impostor.Asset
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
