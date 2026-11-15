//go:build js && wasm

package browser

import (
	"github.com/karty-game/karty-webgpu/internal/spritepaint"
)

// Check the complete incoming page set before any frame uploads occur.
func (driver *spriteFeature) validateSpritePages(frame *spritepaint.Frame) error {
	for _, source := range frame.Textures {
		if _, exists := driver.sprites.pages[source]; exists {
			continue
		}

		if _, err := driver.preparedPageFormat(source); err != nil {
			return err
		}
	}

	return nil
}

func (driver *spriteFeature) updateSpritePages(frame *spritepaint.Frame) error {
	gpu := driver.sprites
	for _, source := range frame.Textures {
		page, ok := gpu.pages[source]
		if !ok {
			var err error

			page, err = driver.uploadSpritePage(source)
			if err != nil {
				return err
			}
		}

		page.used = frame.BatchesRevision
		gpu.pages[source] = page
	}

	for source, page := range gpu.pages {
		if page.used != frame.BatchesRevision {
			page.texture.Call("destroy")
			delete(gpu.pages, source)
		}
	}

	return nil
}

func (driver *spriteFeature) uploadSpritePage(page *spritepaint.Texture) (uiPage, error) {
	finishScope := driver.beginSpriteResources()
	defer finishScope()

	return driver.uploadPreparedPage(page, driver.sprites.pipeline, driver.sprites.sampler)
}
