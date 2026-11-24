package engine

import (
	"github.com/karty-game/ashura/internal/texturedata"
	"github.com/karty-game/ashura/pkg/texture"
)

type texturePreparer interface {
	PrepareTexture(image *texturedata.Texture) error
}

// PrepareTexture uploads one owned, immutable texture and all its prepared mip
// levels through the active backend. Repeated calls reuse the cached GPU image.
// Call it from the app's loop goroutine to distribute uploads across updates.
// Before Run opens a backend, it leaves the existing first-use upload behavior
// unchanged. It performs no image decoding, repacking or GPU completion wait.
func (app *App) PrepareTexture(image *texture.Texture) error {
	if app == nil || app.closed {
		return errAppClosed
	}

	if _, owned := app.textures[image]; !owned || !texturedata.Valid(image) {
		return errTextureOwner
	}

	if preparer, ok := app.driver.(texturePreparer); ok {
		return preparer.PrepareTexture(image)
	}

	return nil
}
