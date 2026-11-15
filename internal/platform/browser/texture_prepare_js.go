//go:build js && wasm

package browser

import "github.com/karty-game/karty-webgpu/internal/texturedata"

// PrepareTexture uses the existing cached upload path before first draw.
func (driver *Driver) PrepareTexture(source *texturedata.Texture) error {
	_, err := driver.textureView(source)

	return err
}
