//nolint:mnd // Scrollbar widths, hit slop and minimum thumb size are authored UI metrics.
package ui

func scrollbarGeometry(node *Node) (Rect, Rect) {
	track := Rect{X: node.rect.X + node.rect.W - 12, Y: node.rect.Y + 4, W: 8, H: max(0, node.rect.H-8)}
	viewport := node.rect.Inset(node.style.Padding).H
	height := min(track.H, max(24, track.H*viewport/max(1, node.contentHeight)))
	positionY := track.Y + node.offset/max(1, scrollExtent(node))*(track.H-height)

	return track, Rect{X: track.X + 3, Y: positionY, W: 3, H: height}
}

func (v *View) beginScrollbar(event Event, node *Node) bool {
	if scrollExtent(node) <= 0 || !modalAvailable(node, v) || !v.inScope(node) {
		return false
	}

	track, thumb := scrollbarGeometry(node)

	target := Rect{X: track.X - 3, Y: track.Y, W: track.W + 6, H: track.H}.Intersect(node.clip)
	if !target.Contains(event.X, event.Y) {
		return false
	}

	if event.Y < thumb.Y || event.Y > thumb.Y+thumb.H {
		fraction := (event.Y - track.Y - thumb.H/2) / max(1, track.H-thumb.H)
		v.setScrollOffset(node, fraction*scrollExtent(node))
	}

	v.capture = &capture{
		node:        node,
		pointer:     event.Pointer,
		device:      event.Device,
		scrollbar:   true,
		positionY:   event.Y,
		scrollStart: node.offset,
	}
	node.damage(false)

	v.pending.Consumed = true
	v.pending.Capture = true
	v.pending.Pointer = event.Pointer

	return true
}

func (v *View) moveScrollbar(child *capture, event Event) {
	track, thumb := scrollbarGeometry(child.node)
	distance := event.Y - child.positionY
	v.setScrollOffset(child.node, child.scrollStart+distance/max(1, track.H-thumb.H)*scrollExtent(child.node))
}
