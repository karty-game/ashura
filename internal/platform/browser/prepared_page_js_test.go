//go:build js && wasm

package browser

import (
	"errors"
	"syscall/js"
	"testing"

	"github.com/karty-game/ashura/internal/spritepaint"
	"github.com/karty-game/ashura/internal/texturedata"
	"github.com/karty-game/ashura/internal/uipaint"
)

func TestPreparedPageRejectsUnsupportedBatchBeforeGPUWrites(t *testing.T) {
	t.Parallel()

	features := js.Global().Get("Set").New()
	// No GPU methods or queue: the entire page set must fail before issuing GPU calls.
	driver := &Driver{device: jsObject(map[string]any{"features": features})}
	plain := &uipaint.Texture{Width: 4, Height: 4, Pixels: make([]byte, 64)}
	compressed := &uipaint.Texture{Width: 4, Height: 4, Format: texturedata.BC7, Pixels: make([]byte, 16)}
	sprites := &spriteFeature{Driver: driver, sprites: &spriteGPU{pages: make(map[*spritepaint.Texture]uiPage)}}

	frame := &spritepaint.Frame{Revision: 1, BatchesRevision: 1, Textures: []*spritepaint.Texture{plain, compressed}, Width: 640}
	if err := sprites.updateSprites(frame); !errors.Is(err, errTextureUpload) {
		t.Fatal("unsupported sprite batch was not rejected before GPU writes", err)
	}

	bridge := &browserUIBridge{Driver: driver, uiGPU: &uiGPU{pages: make(map[*uipaint.Texture]uiPage)}}

	uiFrame := &uipaint.Frame{
		Revision: 1,
		Width:    640,
		Batches:  []uipaint.Batch{{Count: 1, Texture: plain}, {Count: 1, Texture: compressed}},
	}
	if err := bridge.updateUI(uiFrame); !errors.Is(err, errTextureUpload) {
		t.Fatal("unsupported UI batch was not rejected before GPU writes", err)
	}

	if len(sprites.sprites.pages) != 0 || len(bridge.uiGPU.pages) != 0 {
		t.Fatal("rejected page set partially entered the residency cache")
	}

	features.Call("add", textureCompressionBC)

	driver.compression = enabledCompression(driver.device)

	format, err := driver.preparedPageFormat(compressed)
	if err != nil || format != "bc7-rgba-unorm" {
		t.Fatal("enabled page changed linear GPU sampling semantics", format, err)
	}

	compressed.Pixels = compressed.Pixels[:15]
	if _, err := driver.preparedPageFormat(compressed); !errors.Is(err, errTextureUpload) {
		t.Fatal("truncated block page admitted", err)
	}
}
