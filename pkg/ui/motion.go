//nolint:mnd // Interaction durations and press shape are explicit presentation defaults.
package ui

import "time"

// MotionTheme controls bounded interaction transitions on buttons and inputs.
// Zero durations make the corresponding transition immediate. Disabled turns
// off interaction transitions and ripples; system reduced motion also applies.
type MotionTheme struct {
	Disabled                                 bool
	HoverDuration, PressDuration             time.Duration
	FocusDuration, ReleaseDuration           time.Duration
	RippleExpandDuration, RippleFadeDuration time.Duration
}

func defaultMotionTheme() MotionTheme {
	return MotionTheme{
		HoverDuration: 120 * time.Millisecond, PressDuration: 80 * time.Millisecond,
		FocusDuration: 160 * time.Millisecond, ReleaseDuration: 160 * time.Millisecond,
		RippleExpandDuration: 320 * time.Millisecond, RippleFadeDuration: 180 * time.Millisecond,
	}
}

func validateMotionTheme(theme MotionTheme) error {
	for _, duration := range []time.Duration{
		theme.HoverDuration, theme.PressDuration, theme.FocusDuration, theme.ReleaseDuration,
		theme.RippleExpandDuration, theme.RippleFadeDuration,
	} {
		if duration < 0 || duration > 2*time.Second {
			return ErrBounds
		}
	}

	return nil
}

type controlPose struct {
	fill, foreground, border Color
	radius, width, focus     float32
}

type controlMotion struct {
	owner                       *View
	initialized, listed, moving bool
	value, from, target         controlPose
	started, duration, last     time.Duration
	ripple                      controlRippleState
}

func (v *View) animateControl(node *Node, fill, foreground, border Color, radius, width float32) (Color, Color, Color, float32, float32) {
	if node.kind != Button && node.kind != ToggleButton && node.kind != Input {
		return fill, foreground, border, radius, width
	}

	state := v.ensureControlMotion(node)

	target := controlPose{fill: fill, foreground: foreground, border: border, radius: radius, width: width}
	if node.kind == Input && v.focus == node {
		target.focus = 1
	}

	if node.kind != Input && v.pressed == node && !node.readOnly {
		target.radius *= 0.85
	}

	if v.focus == node && v.focusVisible {
		state.value.border, state.value.width = target.border, target.width
	}

	if !state.initialized || !v.controlMotionEnabled(node) {
		state.value, state.from, state.target = target, target, target
		state.initialized, state.moving = true, false
		state.last = v.now
	} else if target != state.target {
		state.from, state.target = state.value, target
		state.started, state.duration, state.last = v.now, v.controlMotionDuration(node, target, state.value), v.now

		state.moving = state.duration > 0 && state.from != target
		if state.moving {
			v.scheduleControlMotion(node)
		} else {
			state.value = target
		}
	}

	return state.value.fill, state.value.foreground, state.value.border, state.value.radius, state.value.width
}

func (v *View) ensureControlMotion(node *Node) *controlMotion {
	state := &node.motion
	if state.owner != v {
		*state = controlMotion{owner: v}
	}

	return state
}

func (v *View) controlMotionEnabled(node *Node) bool {
	return !v.reducedMotion && !v.theme.Motion.Disabled && !inheritedDisabled(node) && (!node.readOnly || node.kind == Input)
}

func (v *View) controlMotionDuration(node *Node, target, previous controlPose) time.Duration {
	theme := v.theme.Motion
	if v.pressed == node {
		return theme.PressDuration
	}

	if target.focus != previous.focus || v.focus == node {
		return theme.FocusDuration
	}

	if v.hover == node {
		return theme.HoverDuration
	}

	return theme.ReleaseDuration
}

func (v *View) controlFocus(node *Node) float32 {
	if node.motion.owner != v || !node.motion.initialized {
		return 0
	}

	return node.motion.value.focus
}

func controlEase(fraction float32) float32 {
	remaining := 1 - max(0, min(1, fraction))

	return 1 - remaining*remaining*remaining
}

func controlBlend(first, last controlPose, fraction float32) controlPose {
	result := controlPose{}
	for channel := range result.fill {
		result.fill[channel] = first.fill[channel] + (last.fill[channel]-first.fill[channel])*fraction
		result.foreground[channel] = first.foreground[channel] + (last.foreground[channel]-first.foreground[channel])*fraction
		result.border[channel] = first.border[channel] + (last.border[channel]-first.border[channel])*fraction
	}

	result.radius = first.radius + (last.radius-first.radius)*fraction
	result.width = first.width + (last.width-first.width)*fraction
	result.focus = first.focus + (last.focus-first.focus)*fraction

	return result
}
