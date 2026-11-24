//go:build !js

package lightmapbuild

import (
	"github.com/karty-game/ashura/internal/meshdata"
	"github.com/karty-game/ashura/internal/modeldata"
	"github.com/karty-game/ashura/internal/xatlas"
)

func decodeGeometry(data []byte) (modeldata.Document, error) {
	model, err := modeldata.Decode(data)
	if err != nil {
		return modeldata.Document{}, err
	}

	for _, part := range model.Parts {
		if _, err := meshdata.FromOwned(part.Vertices, part.Indices); err != nil {
			return modeldata.Document{}, err
		}
	}

	return model, nil
}

func pageGeometry(source modeldata.Document, page xatlas.Page) (modeldata.Document, [][]byte, xatlas.Atlas) {
	model := productionPageModel(source, page.PartIndices)
	uvs := remapModel(&model, page.Atlas)

	return model, uvs, page.Atlas
}
