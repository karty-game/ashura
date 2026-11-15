package draw2d

import (
	"fmt"

	"github.com/karty-game/karty-webgpu/internal/spritepaint"
)

// Screen owns a flat retained sprite layer in framebuffer pixels.
// A Screen must not be copied after first use.
type Screen struct {
	slots                          []*Sprite
	free, dirty                    []int
	ordered                        []*Sprite
	pages                          map[*spritepaint.Texture]int
	imageBytes, count              int
	sequence                       uint64
	width, height                  int
	clip                           rectangle
	closed, orderDirty, batchDirty bool
	pagesDirty                     bool
	stats                          Stats
	frame                          spritepaint.Frame
}

// Stats reports current residency and cumulative retained preparation work.
type Stats struct {
	Sprites, Pages, ImageBytes int
	PackedRecords, BatchBuilds uint64
}

func NewScreen() *Screen { return &Screen{pages: make(map[*spritepaint.Texture]int)} }

func (screen *Screen) Stats() Stats {
	stats := screen.stats
	stats.Sprites, stats.Pages, stats.ImageBytes = screen.count, len(screen.pages), screen.imageBytes

	return stats
}

func (screen *Screen) NewSprite(image *Image) *Sprite {
	screen.check()

	if !validImage(image) {
		panic("draw2d: NewSprite requires an admitted image")
	}

	if screen.count >= MaxSprites {
		panic(fmt.Sprintf("draw2d: maximum %d sprites exceeded", MaxSprites))
	}

	screen.replacePage(nil, image)
	screen.sequence++

	sprite := &Sprite{
		screen:   screen,
		image:    image,
		sequence: screen.sequence,
		scaleX:   1,
		scaleY:   1,
		tint:     [3]float32{1, 1, 1},
		opacity:  1,
		visible:  true,
	}
	if len(screen.free) > 0 {
		sprite.slot = screen.free[len(screen.free)-1]
		screen.free = screen.free[:len(screen.free)-1]
		screen.slots[sprite.slot] = sprite
	} else {
		sprite.slot = len(screen.slots)
		screen.slots = append(screen.slots, sprite)
		screen.frame.Data = append(screen.frame.Data, make([]byte, spritepaint.Stride)...)
	}

	screen.count++
	screen.orderDirty, screen.batchDirty = true, true

	sprite.changed(true)

	return sprite
}

func (screen *Screen) SetClip(x, y, width, height int) *Screen {
	screen.check()

	clip := makeRectangle(x, y, width, height)
	if screen.clip != clip {
		screen.clip = clip
		screen.invalidateClips()
	}

	return screen
}
func (screen *Screen) ClearClip() *Screen {
	screen.check()

	if screen.clip.set {
		screen.clip = rectangle{}
		screen.invalidateClips()
	}

	return screen
}
func (screen *Screen) Clip() (int, int, int, int, bool) {
	return screen.clip.x, screen.clip.y, screen.clip.w, screen.clip.h, screen.clip.set
}

// Resize updates drawable coordinates before game callbacks, without packing.
func (screen *Screen) Resize(width, height int) *Screen {
	screen.check()

	if width < 1 || height < 1 || width > MaxViewportSize || height > MaxViewportSize {
		panic("draw2d: framebuffer dimensions must be 1..16384")
	}

	if screen.width != width || screen.height != height {
		screen.width, screen.height = width, height
		screen.invalidateClips()
	}

	return screen
}
func (screen *Screen) Size() (int, int) { return screen.width, screen.height }
func (screen *Screen) FromLogical(logicalX, logicalY, logicalWidth, logicalHeight float32) (float32, float32) {
	screen.check()
	finite("logical coordinates", logicalX, logicalY, logicalWidth, logicalHeight)

	if logicalWidth <= 0 || logicalHeight <= 0 {
		panic("draw2d: logical dimensions must be positive")
	}

	return logicalX * (float32(screen.width) / logicalWidth), logicalY * (float32(screen.height) / logicalHeight)
}

// Close releases screen references and makes subsequent mutations invalid.
func (screen *Screen) Close() {
	if screen == nil || screen.closed {
		return
	}

	screen.closed = true
	for _, sprite := range screen.slots {
		if sprite != nil {
			sprite.destroyed = true
			sprite.image = nil
		}
	}

	screen.slots = nil
	screen.ordered = nil
	screen.dirty = nil
	screen.free = nil
	screen.pages = nil
	screen.count, screen.imageBytes = 0, 0
	screen.frame = spritepaint.Frame{}
}
