package level

import (
	"github.com/karty-game/ashura/pkg/engine"
	"github.com/karty-game/ashura/pkg/material"
	"github.com/karty-game/ashura/pkg/texture"
)

func (chunk *LoadedChunk) loadRGBMaterials(page Page) error {
	image, err := chunk.retainedImage(page.Maps[0].SHA256)
	if err != nil {
		return err
	}

	maps := &texture.RGBLightmap{Texture: image, Scale: page.Scale}
	if page.Encoding == "rgbe8" {
		maps.Encoding = texture.RNMRGBE
	}

	for _, source := range page.Materials {
		options := material.Options{
			Color:         engine.Color{R: source.Color[0], G: source.Color[1], B: source.Color[2], A: source.Color[3]},
			RGBLightmap:   maps,
			CullBackfaces: !source.DoubleSided,
		}
		if source.Albedo != nil {
			options.AlbedoTexture, err = chunk.retainedImage(source.Albedo.SHA256)
			if err != nil {
				return err
			}
		}

		last := &chunk.pages[len(chunk.pages)-1]
		last.standard = append(last.standard, material.Standard(options))
	}

	return nil
}
