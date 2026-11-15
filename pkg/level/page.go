package level

import (
	"github.com/karty-game/karty-webgpu/internal/leveldata"
	"github.com/karty-game/karty-webgpu/pkg/engine"
	"github.com/karty-game/karty-webgpu/pkg/material"
	"github.com/karty-game/karty-webgpu/pkg/texture"
)

func (chunk *LoadedChunk) loadPage(page preparedPage) error {
	asset, err := page.asset.LoadLightmapped(chunk.loader.app, page.layout)
	if err != nil {
		return err
	}

	chunk.pages = append(chunk.pages, loadedPage{model: asset})
	if page.description.Representation == leveldata.RGBDiffuse {
		return chunk.loadRGBMaterials(page.description)
	}

	maps := &texture.RNMLightmap{Scale: page.description.Scale}
	if page.description.Encoding == "rgbe8" {
		maps.Encoding = texture.RNMRGBE
	}

	for index, ref := range page.description.Maps {
		maps.Textures[index], err = chunk.retainedImage(ref.SHA256)
		if err != nil {
			return err
		}
	}

	for _, source := range page.description.Materials {
		options := material.SourceOptions{
			Color:         engine.Color{R: source.Color[0], G: source.Color[1], B: source.Color[2], A: source.Color[3]},
			RNMLightmap:   maps,
			CullBackfaces: !source.DoubleSided,
		}
		if source.Albedo != nil {
			options.AlbedoTexture, err = chunk.retainedImage(source.Albedo.SHA256)
			if err != nil {
				return err
			}
		}

		if source.SSBump != nil {
			options.SSBumpTexture, err = chunk.retainedImage(source.SSBump.SHA256)
			if err != nil {
				return err
			}
		}

		last := &chunk.pages[len(chunk.pages)-1]
		last.materials = append(last.materials, material.Source(options))
	}

	return nil
}
func (chunk *LoadedChunk) retainedImage(key string) (*texture.Texture, error) {
	shared := chunk.loader.textures[key]
	if !chunk.textureSet[key] || shared == nil {
		return nil, leveldata.ErrFormat
	}

	return shared.image, nil
}
