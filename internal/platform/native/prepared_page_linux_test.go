//go:build linux

package native

import (
	"errors"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/karty-game/ashura/internal/spritepaint"
	"github.com/karty-game/ashura/internal/texturedata"
	"github.com/karty-game/ashura/internal/uipaint"
)

func TestPreparedPageRejectsUnsupportedBatchBeforeGPUWrites(t *testing.T) {
	t.Parallel()

	// Nil device and queue prove preflight rejects the entire set before any GPU calls.
	driver := &Driver{}
	plain := &uipaint.Texture{Width: 4, Height: 4, Pixels: make([]byte, 64)}
	compressed := &uipaint.Texture{Width: 4, Height: 4, Format: texturedata.BC7, Pixels: make([]byte, 16)}
	sprites := &spriteFeature{Driver: driver, sprites: &spriteGPU{pages: make(map[*spritepaint.Texture]uiPage)}}

	frame := &spritepaint.Frame{Revision: 1, BatchesRevision: 1, Textures: []*spritepaint.Texture{plain, compressed}, Width: 640}
	if err := sprites.updateSprites(frame); !errors.Is(err, errNativeCompression) {
		t.Fatal("unsupported sprite batch was not rejected before GPU writes", err)
	}

	bridge := &nativeUIHost{Driver: driver, uiGPU: &uiGPU{pages: make(map[*uipaint.Texture]uiPage)}}

	uiFrame := &uipaint.Frame{
		Revision: 1,
		Width:    640,
		Batches:  []uipaint.Batch{{Count: 1, Texture: plain}, {Count: 1, Texture: compressed}},
	}
	if err := bridge.updateUI(uiFrame); !errors.Is(err, errNativeCompression) {
		t.Fatal("unsupported UI batch was not rejected before GPU writes", err)
	}

	if len(sprites.sprites.pages) != 0 || len(bridge.uiGPU.pages) != 0 {
		t.Fatal("rejected page set partially entered the residency cache")
	}

	driver.compression.bc = true

	format, err := driver.preparedPageFormat(compressed)
	if err != nil || format != gputypes.TextureFormatBC7RGBAUnorm {
		t.Fatal("enabled page changed linear GPU sampling semantics", format, err)
	}

	compressed.Pixels = compressed.Pixels[:15]
	if _, err := driver.preparedPageFormat(compressed); !errors.Is(err, errNativeCompression) {
		t.Fatal("truncated block page admitted", err)
	}
}
