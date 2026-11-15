package draw2d

import "math"

type rectangle struct {
	x, y, w, h int
	set        bool
}

func makeRectangle(left, top, width, height int) rectangle {
	// Signed 32-bit edges remain identical on browser and native hosts.
	if width < 0 || height < 0 || left < math.MinInt32 || left > math.MaxInt32 || top < math.MinInt32 || top > math.MaxInt32 ||
		width > math.MaxInt32 ||
		height > math.MaxInt32 ||
		left > math.MaxInt32-width ||
		top > math.MaxInt32-height {
		panic("draw2d: clip requires nonnegative extents and signed 32-bit edges")
	}

	return rectangle{x: left, y: top, w: width, h: height, set: true}
}

func intersect(left, right rectangle) rectangle {
	if !right.set {
		return left
	}

	leftEdge, topEdge := max(left.x, right.x), max(left.y, right.y)

	return rectangle{
		x:   leftEdge,
		y:   topEdge,
		w:   max(0, min(left.x+left.w, right.x+right.w)-leftEdge),
		h:   max(0, min(left.y+left.h, right.y+right.h)-topEdge),
		set: true,
	}
}

func (screen *Screen) clipFor(sprite *Sprite) rectangle {
	clip := intersect(rectangle{w: screen.width, h: screen.height, set: true}, screen.clip)
	clip = intersect(clip, sprite.clip)
	clip.x, clip.y = min(screen.width, max(0, clip.x)), min(screen.height, max(0, clip.y))

	return clip
}
