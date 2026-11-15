package draw2d

import "math"

// Sprite is a retained screen-owned image. Mutations belong to the App loop thread.
// A Sprite must not be copied after creation; retain its pointer instead.
type Sprite struct {
	screen                                         *Screen
	image                                          *Image
	slot                                           int
	sequence                                       uint64
	x, y, scaleX, scaleY, rotation, pivotX, pivotY float32
	tint                                           [3]float32
	opacity                                        float32
	visible, destroyed, dirty, active              bool
	order                                          int32
	clip                                           rectangle
}

func finite(property string, values ...float32) {
	for _, value := range values {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			panic("draw2d: " + property + " must be finite")
		}
	}
}

func (s *Sprite) SetImage(image *Image) *Sprite {
	s.check()

	if !validImage(image) {
		panic("draw2d: SetImage requires an admitted image")
	}

	if s.image == image {
		return s
	}

	pageChanged := s.image.page != image.page
	s.screen.replacePage(s.image, image)
	s.image = image
	s.changed(pageChanged)

	return s
}
func (s *Sprite) SetPosition(x, y float32) *Sprite {
	s.check()
	transformValues("position", x, y)

	if s.x != x || s.y != y {
		s.x, s.y = x, y
		s.changed(false)
	}

	return s
}
func (s *Sprite) SetScale(x, y float32) *Sprite {
	s.check()
	transformValues("scale", x, y)

	if s.scaleX != x || s.scaleY != y {
		s.scaleX, s.scaleY = x, y
		s.changed(false)
	}

	return s
}
func (s *Sprite) SetRotation(radians float32) *Sprite {
	s.check()
	finite("rotation", radians)

	if s.rotation != radians {
		s.rotation = radians
		s.changed(false)
	}

	return s
}
func (s *Sprite) SetPivot(x, y float32) *Sprite {
	s.check()
	transformValues("pivot", x, y)

	if s.pivotX != x || s.pivotY != y {
		s.pivotX, s.pivotY = x, y
		s.changed(false)
	}

	return s
}
func (s *Sprite) SetTint(r, g, b float32) *Sprite {
	s.check()
	finite("tint", r, g, b)

	tint := [3]float32{clamp(r), clamp(g), clamp(b)}
	if s.tint != tint {
		s.tint = tint
		s.changed(false)
	}

	return s
}
func (s *Sprite) SetOpacity(opacity float32) *Sprite {
	s.check()
	finite("opacity", opacity)

	opacity = clamp(opacity)
	if s.opacity != opacity {
		s.opacity = opacity
		s.changed(false)
	}

	return s
}
func (s *Sprite) SetVisible(visible bool) *Sprite {
	s.check()

	if s.visible != visible {
		s.visible = visible
		s.changed(false)
	}

	return s
}
func (s *Sprite) SetOrder(order int32) *Sprite {
	s.check()

	if s.order != order {
		s.order = order
		s.screen.orderDirty = true
		s.screen.batchDirty = true
	}

	return s
}
func (s *Sprite) SetClip(x, y, width, height int) *Sprite {
	s.check()

	clip := makeRectangle(x, y, width, height)
	if s.clip != clip {
		s.clip = clip
		s.changed(true)
	}

	return s
}
func (s *Sprite) ClearClip() *Sprite {
	s.check()

	if s.clip.set {
		s.clip = rectangle{}
		s.changed(true)
	}

	return s
}

// Destroy is idempotent. Other subsequent mutations panic.
func (s *Sprite) Destroy() {
	if s == nil || s.destroyed {
		return
	}

	s.check()
	s.destroyed = true
	s.screen.releasePage(s.image.page)
	s.screen.count--
	s.screen.orderDirty, s.screen.batchDirty = true, true
	s.changed(true)
	s.screen.slots[s.slot] = nil
	s.screen.free = append(s.screen.free, s.slot)
	s.image = nil
}
func (s *Sprite) Image() *Image                     { return s.image }
func (s *Sprite) Position() (float32, float32)      { return s.x, s.y }
func (s *Sprite) Scale() (float32, float32)         { return s.scaleX, s.scaleY }
func (s *Sprite) Pivot() (float32, float32)         { return s.pivotX, s.pivotY }
func (s *Sprite) Rotation() float32                 { return s.rotation }
func (s *Sprite) Tint() (float32, float32, float32) { return s.tint[0], s.tint[1], s.tint[2] }
func (s *Sprite) Opacity() float32                  { return s.opacity }
func (s *Sprite) Visible() bool                     { return s.visible }
func (s *Sprite) Order() int32                      { return s.order }
func (s *Sprite) Clip() (int, int, int, int, bool) {
	return s.clip.x, s.clip.y, s.clip.w, s.clip.h, s.clip.set
}
func clamp(value float32) float32 { return max(0, min(1, value)) }
