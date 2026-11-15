package draw2d

func transformValues(property string, values ...float32) {
	finite(property, values...)

	for _, value := range values {
		if value < -MaxTransformValue || value > MaxTransformValue {
			panic("draw2d: " + property + " exceeds absolute transform limit 16777216")
		}
	}
}

func (s *Sprite) check() {
	if s == nil || s.destroyed || s.screen == nil || s.screen.closed {
		panic("draw2d: mutation of destroyed sprite or closed screen")
	}

	if s.slot < 0 || s.slot >= len(s.screen.slots) || s.screen.slots[s.slot] != s {
		panic("draw2d: mutation through a copied or stale sprite handle")
	}
}

func (s *Sprite) changed(batches bool) {
	if !s.dirty {
		s.dirty = true
		s.screen.dirty = append(s.screen.dirty, s.slot)
	}

	s.screen.batchDirty = s.screen.batchDirty || batches
}
