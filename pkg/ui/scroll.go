package ui

import "time"

const wheelEaseDuration = 140 * time.Millisecond

type scrollMotion struct {
	node         *Node
	from, target float32
	started      time.Duration
}

// SetReducedMotion makes discrete wheel input immediate and finishes active easing.
// Platform wrappers may forward system preferences; callers can also opt out explicitly.
func (v *View) SetReducedMotion(reduced bool) {
	if v.reducedMotion == reduced {
		return
	}

	v.reducedMotion = reduced
	if reduced {
		v.finishControlMotion()

		for _, motion := range v.scrolls {
			if motion.node.view == v && motion.node.style.Scroll && modalAvailable(motion.node, v) && v.inScope(motion.node) {
				v.setScrollOffset(motion.node, motion.target)
			}
		}

		v.scrolls = v.scrolls[:0]
	}
}

func (v *View) ReducedMotion() bool { return v.reducedMotion }

// ScrollOffset exposes the displayed vertical offset in logical pixels.
func (node *Node) ScrollOffset() float32 { return node.offset }

func scrollExtent(node *Node) float32 {
	return max(0, node.contentHeight-node.rect.Inset(node.style.Padding).H)
}

func (v *View) setScrollOffset(node *Node, offset float32) {
	offset = max(0, min(scrollExtent(node), offset))
	if offset != node.offset {
		node.offset = offset
		node.damage(false)

		v.placementDirty = true
	}
}

// Direct manipulation and keyboard scrolling stay synchronous with the input.
func (v *View) scrollBy(node *Node, delta float32) bool {
	for parent := node; parent != nil; parent = parent.parent {
		if !parent.style.Scroll || !modalAvailable(parent, v) || !v.inScope(parent) {
			continue
		}

		offset := max(0, min(scrollExtent(parent), parent.offset+delta))
		if offset != parent.offset {
			v.stopScroll(parent)
			v.setScrollOffset(parent, offset)

			return true
		}
	}

	return false
}

func (v *View) smoothScrollBy(node *Node, delta float32) bool {
	for parent := node; parent != nil; parent = parent.parent {
		if !parent.style.Scroll || !modalAvailable(parent, v) || !v.inScope(parent) || scrollExtent(parent) <= 0 {
			continue
		}

		// An overflowing viewport owns wheel input, including at its boundaries.
		// Queued motion reaching an edge must not send the rest of a wheel burst
		// into the outer page before the viewport has even reached its target.
		if v.reducedMotion {
			v.stopScroll(parent)
			v.setScrollOffset(parent, parent.offset+delta)

			return true
		}

		target := v.scrollTarget(parent)
		if (target-parent.offset)*delta < 0 {
			v.stopScroll(parent)

			target = parent.offset
		}

		next := max(0, min(scrollExtent(parent), target+delta))
		if next == target {
			return true
		}

		v.stopScroll(parent)
		v.scrolls = append(v.scrolls, scrollMotion{node: parent, from: parent.offset, target: next, started: v.now})

		return true
	}

	return false
}

func (v *View) scrollTarget(node *Node) float32 {
	for _, motion := range v.scrolls {
		if motion.node == node {
			return motion.target
		}
	}

	return node.offset
}

func (v *View) stopScroll(node *Node) {
	for index := 0; index < len(v.scrolls); {
		if v.scrolls[index].node == node {
			copy(v.scrolls[index:], v.scrolls[index+1:])
			v.scrolls = v.scrolls[:len(v.scrolls)-1]
		} else {
			index++
		}
	}
}

func (v *View) advanceScroll(now time.Duration) {
	active := v.scrolls[:0]
	for _, motion := range v.scrolls {
		node := motion.node
		if node.view != v || !node.style.Scroll || !modalAvailable(node, v) || !v.inScope(node) {
			continue
		}

		elapsed := max(time.Duration(0), now-motion.started)
		fraction := min(float32(1), float32(elapsed)/float32(wheelEaseDuration))
		remaining := 1 - fraction
		target := max(0, min(scrollExtent(node), motion.target))

		offset := motion.from + (target-motion.from)*(1-remaining*remaining*remaining)
		if fraction == 1 || abs(target-offset) < 0.05 {
			offset = target
		} else {
			active = append(active, motion)
		}

		v.setScrollOffset(node, offset)
	}

	v.scrolls = active
}
