package draw2d

import (
	"fmt"

	"github.com/karty-game/ashura/internal/spritepaint"
)

func (screen *Screen) check() {
	if screen == nil || screen.closed {
		panic("draw2d: mutation of closed screen")
	}
}

func (screen *Screen) replacePage(old, next *Image) {
	if old != nil && old.page == next.page {
		return
	}

	bytes, pages := screen.imageBytes, len(screen.pages)
	if old != nil && screen.pages[old.page] == 1 {
		bytes -= len(old.page.Pixels)
		pages--
	}

	if screen.pages[next.page] == 0 {
		bytes += len(next.page.Pixels)
		pages++
	}

	if bytes > MaxImageBytes {
		panic(fmt.Sprintf("draw2d: maximum %d image bytes exceeded", MaxImageBytes))
	}

	if pages > MaxPages {
		panic(fmt.Sprintf("draw2d: maximum %d pages exceeded", MaxPages))
	}

	if old != nil {
		screen.releasePage(old.page)
	}

	if screen.pages[next.page] == 0 {
		screen.imageBytes += len(next.page.Pixels)
	}

	screen.pages[next.page]++
	screen.pagesDirty = true
}

func (screen *Screen) releasePage(page *spritepaint.Texture) {
	screen.pages[page]--
	if screen.pages[page] == 0 {
		delete(screen.pages, page)
		screen.imageBytes -= len(page.Pixels)
		screen.pagesDirty = true
	}
}

func (screen *Screen) invalidateClips() {
	for _, sprite := range screen.slots {
		if sprite != nil && !sprite.destroyed {
			sprite.changed(true)
		}
	}

	screen.batchDirty = true
}
