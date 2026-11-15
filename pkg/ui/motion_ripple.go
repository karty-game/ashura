package ui

import (
	"math"
	"time"
)

type controlRippleState struct {
	active, released      bool
	x, y, growth, opacity float32
	started, releaseAt    time.Duration
	expand, fade          time.Duration
}

func (v *View) startControlRipple(node *Node, positionX, positionY float32) {
	if node == nil || (node.kind != Button && node.kind != ToggleButton) || node.readOnly ||
		!v.controlMotionVisible(node) || !v.controlMotionEnabled(node) || v.theme.Motion.RippleExpandDuration == 0 {
		return
	}

	state := v.ensureControlMotion(node)
	state.last = v.now
	state.ripple = controlRippleState{
		active: true, opacity: 1,
		x:       max(0, min(1, (positionX-node.rect.X)/max(1, node.rect.W))),
		y:       max(0, min(1, (positionY-node.rect.Y)/max(1, node.rect.H))),
		started: v.now, expand: v.theme.Motion.RippleExpandDuration, fade: v.theme.Motion.RippleFadeDuration,
	}
	v.scheduleControlMotion(node)
	node.damage(false)
}

func (v *View) releaseControlRipple(node *Node) {
	if node == nil || node.motion.owner != v || !node.motion.ripple.active || node.motion.ripple.released {
		return
	}

	state := &node.motion.ripple
	state.released = true
	state.releaseAt = max(v.now, state.started+state.expand*35/100)
	v.scheduleControlMotion(node)
	node.damage(false)
}

func (v *View) cancelControlRipple(node *Node) {
	if node == nil || node.motion.owner != v || !node.motion.ripple.active {
		return
	}

	node.motion.ripple = controlRippleState{}
	node.damage(false)
}

func (v *View) advanceControlRipple(state *controlMotion, now time.Duration) bool {
	ripple := &state.ripple
	if !ripple.active {
		return false
	}

	previousGrowth, previousOpacity := ripple.growth, ripple.opacity
	elapsed := max(time.Duration(0), now-ripple.started)

	ripple.growth = controlEase(min(float32(1), float32(elapsed)/float32(ripple.expand)))
	if ripple.released && now >= ripple.releaseAt {
		if ripple.fade == 0 {
			ripple.opacity = 0
		} else {
			fraction := min(float32(1), float32(now-ripple.releaseAt)/float32(ripple.fade))
			ripple.opacity = 1 - controlEase(fraction)
		}

		if ripple.opacity == 0 {
			ripple.active = false
		}
	}

	return ripple.growth != previousGrowth || ripple.opacity != previousOpacity
}

// controlRipple returns logical origin/radius and a 0..1 opacity multiplier.
// RipplePart supplies the actual paint color and its maximum opacity.
func (v *View) controlRipple(node *Node) (float32, float32, float32, float32, bool) {
	state := node.motion.ripple
	if node.motion.owner != v || !state.active || !v.controlMotionEnabled(node) {
		return 0, 0, 0, 0, false
	}

	width, height := node.rect.W, node.rect.H
	positionX, positionY := node.rect.X+state.x*width, node.rect.Y+state.y*height
	farX, farY := max(state.x, 1-state.x)*width, max(state.y, 1-state.y)*height
	maximum := float32(math.Hypot(float64(farX), float64(farY)))
	radius := min(float32(2), maximum) + max(0, maximum-2)*state.growth

	return positionX, positionY, radius, state.opacity, state.opacity > 0
}
