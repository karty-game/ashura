package ui

import "time"

func (v *View) scheduleControlMotion(node *Node) {
	if node.motion.listed {
		return
	}

	node.motion.listed = true
	v.controlMotions = append(v.controlMotions, node)
}

func (v *View) advanceControlMotion(now time.Duration) {
	active := v.controlMotions[:0]
	for _, node := range v.controlMotions {
		state := &node.motion
		if state.owner != v {
			continue
		}

		if !v.controlMotionVisible(node) || !v.controlMotionEnabled(node) {
			state.initialized, state.listed, state.moving = false, false, false
			state.ripple = controlRippleState{}

			node.damage(false)

			continue
		}

		if now < state.last {
			shift := now - state.last
			state.started += shift
			state.ripple.started += shift
			state.ripple.releaseAt += shift
		}

		state.last = now
		changed := advanceControlPose(state, now)

		changed = v.advanceControlRipple(state, now) || changed
		if changed {
			node.damage(false)
		}

		if state.moving || state.ripple.active {
			active = append(active, node)
		} else {
			state.listed = false
		}
	}

	clear(v.controlMotions[len(active):])
	v.controlMotions = active
}

func advanceControlPose(state *controlMotion, now time.Duration) bool {
	if !state.moving {
		return false
	}

	previous := state.value
	elapsed := max(time.Duration(0), now-state.started)
	fraction := min(float32(1), float32(elapsed)/float32(state.duration))

	state.value = controlBlend(state.from, state.target, controlEase(fraction))
	if fraction == 1 {
		state.value = state.target
		state.moving = false
	}

	return state.value != previous
}

func (v *View) controlMotionVisible(node *Node) bool {
	if node.view != v || !node.eligible() || !v.inScope(node) {
		return false
	}

	visible := node.rect.Intersect(node.clip)

	return visible.W > 0 && visible.H > 0
}

func (v *View) finishControlMotion() {
	for _, node := range v.controlMotions {
		state := &node.motion
		if state.owner != v {
			continue
		}

		state.initialized, state.listed, state.moving = false, false, false
		state.ripple = controlRippleState{}

		node.damage(false)
	}

	clear(v.controlMotions)
	v.controlMotions = v.controlMotions[:0]
}
