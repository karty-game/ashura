//nolint:mnd // Tooltip timing, padding and viewport gutters are explicit presentation metrics.
package ui

import (
	"time"
	"unicode/utf8"
)

const (
	tooltipDelay        = 150 * time.Millisecond
	tooltipFadeDuration = 120 * time.Millisecond
)

type tooltipState struct {
	focus                          *Node
	pointerX, pointerY             float32
	pointerActive                  bool
	style                          PartStyle
	node                           *Node
	since                          time.Duration
	opacity                        float32
	ready, visible, blocked, dirty bool
	paint                          Node
	bounds, anchor                 Rect
	width, height                  float32
	theme                          Theme
	text                           string
}

// SetTooltip changes supplementary help. Empty text removes it; focus is never moved.
func (node *Node) SetTooltip(text string) error {
	if len(text) > MaxTextBytes || !utf8.ValidString(text) {
		return invalid("SetTooltip", "TextBytes", len(text), "requires valid UTF-8 within MaxTextBytes")
	}

	if node.tooltip == text {
		return nil
	}

	node.tooltip = text
	if node.view != nil && node.view.tooltip.node == node {
		node.view.tooltip.dirty = true
	}

	return nil
}

func (node *Node) Tooltip() string { return node.tooltip }

func (v *View) updateTooltip(now time.Duration) {
	state := &v.tooltip

	candidate := v.tooltipCandidate()
	if candidate != state.node {
		if state.visible {
			state.dirty = true
		}

		state.node = candidate
		state.visible = false
		state.opacity = 0
		state.blocked = false
		state.ready = false
	}

	if candidate == nil || state.blocked {
		return
	}

	if !state.ready || now < state.since {
		state.dirty = state.dirty || state.visible
		state.visible = false
		state.opacity = 0
		state.since = now
		state.ready = true
	}

	if !state.visible && now-state.since >= tooltipDelay {
		state.visible = true
		state.dirty = true
	}

	if state.visible {
		elapsed := max(time.Duration(0), now-state.since-tooltipDelay)
		fraction := min(float32(1), float32(elapsed)/float32(tooltipFadeDuration))
		remaining := 1 - fraction

		opacity := 1 - remaining*remaining*remaining
		if v.reducedMotion {
			opacity = 1
		}

		if opacity != state.opacity {
			state.opacity = opacity
			state.dirty = true
		}
	}
}

func (v *View) tooltipCandidate() *Node {
	if v.capture != nil || v.popup != nil {
		return nil
	}

	state := &v.tooltip
	if state.pointerActive {
		if state.visible && v.tooltipEligible(state.node) && v.tooltipHoverRegion().Contains(state.pointerX, state.pointerY) {
			return state.node
		}

		if v.tooltipEligible(v.hover) {
			return v.hover
		}
	}

	if v.focusVisible && v.tooltipEligible(v.focus) {
		return v.focus
	}

	return nil
}

func (v *View) tooltipEligible(node *Node) bool {
	if node == nil || node.tooltip == "" || !node.eligible() || !v.inScope(node) {
		return false
	}

	visible := node.rect.Intersect(node.clip)

	return visible.W > 0 && visible.H > 0
}

// dismissTooltip keeps a dismissed stationary target quiet until a new interaction.
func (v *View) dismissTooltip() bool {
	state := &v.tooltip

	visible := state.visible
	if visible {
		state.dirty = true
	}

	state.visible = false
	state.opacity = 0

	state.blocked = true
	if state.node == nil {
		state.node = v.tooltipCandidate()
	}

	return visible
}

func (v *View) resumeTooltip() {
	state := &v.tooltip
	if state.focus != v.focus {
		if v.focusVisible {
			state.pointerActive = false
		}

		state.focus = v.focus
	}

	if state.pointerActive && state.visible && v.tooltipHoverRegion().Contains(state.pointerX, state.pointerY) {
		return
	}

	if state.visible {
		state.dirty = true
	}

	state.visible = false
	state.opacity = 0
	state.blocked = false
	state.ready = false
	state.node = nil
}

func (v *View) tooltipPointer(positionX, positionY float32) {
	v.tooltip.pointerX, v.tooltip.pointerY = positionX, positionY
	v.tooltip.pointerActive = true
	v.tooltip.focus = v.focus
}

func (v *View) tooltipHoverRegion() Rect {
	state := &v.tooltip
	left, top := min(state.anchor.X, state.bounds.X), min(state.anchor.Y, state.bounds.Y)
	right, bottom := max(
		state.anchor.X+state.anchor.W,
		state.bounds.X+state.bounds.W,
	), max(
		state.anchor.Y+state.anchor.H,
		state.bounds.Y+state.bounds.H,
	)

	return Rect{X: left, Y: top, W: right - left, H: bottom - top}
}

func (v *View) paintTooltip() error {
	state := &v.tooltip
	if !state.visible || !v.tooltipEligible(state.node) {
		state.visible = false

		state.dirty = false
		if len(state.paint.paint) == 0 {
			return nil
		}

		state.paint.paint = state.paint.paint[:0]

		return v.writeBlock(&state.paint, v.font.page)
	}

	node := state.node
	anchor := node.rect.Intersect(node.clip)

	style := v.part(node, TooltipPart, PartStyle{Fill: v.theme.Surface, Border: v.theme.Border, Radius: 6})
	if !state.dirty && state.anchor == anchor && state.width == v.width && state.height == v.height && state.theme == v.theme &&
		state.text == node.tooltip && state.style == style {
		return nil
	}

	layoutData, err := v.shape(node.tooltip, v.theme.FontSize, max(1, min(280, v.width-16)-24), true)
	if err != nil {
		return err
	}

	width, height := tooltipSize(layoutData, v.width, v.height)
	positionX := max(0, min(v.width-width, anchor.X+(anchor.W-width)/2))

	positionY := anchor.Y + anchor.H + 8
	if positionY+height > v.height-8 {
		positionY = anchor.Y - height - 8
	}

	positionY = max(0, min(v.height-height, positionY))
	bounds := Rect{X: positionX, Y: positionY, W: width, H: height}
	clip := bounds.Intersect(Rect{W: v.width, H: v.height})
	state.paint.paint = state.paint.paint[:0]
	v.box(&state.paint, bounds, clip, style.Fill, style.Border, style.Radius, 1)
	v.paintLayout(
		&state.paint,
		layoutData,
		bounds.X+12,
		bounds.Y+12,
		clip.Intersect(bounds.Inset(min(12, min(bounds.W, bounds.H)/2))),
		v.theme.Foreground,
	)

	for index := range state.paint.paint {
		state.paint.paint[index].color[3] *= state.opacity
		state.paint.paint[index].border[3] *= state.opacity
	}

	state.bounds = bounds
	state.anchor = anchor
	state.width, state.height = v.width, v.height
	state.theme = v.theme
	state.text = node.tooltip
	state.style = style
	state.dirty = false

	return v.writeBlock(&state.paint, v.font.page)
}

func tooltipSize(layoutData textLayout, width, height float32) (float32, float32) {
	extent := float32(0)
	for _, position := range layoutData.carets {
		extent = max(extent, position.positionX)
	}

	return max(1, min(width, min(280, extent+24))), max(1, min(height, layoutData.height+24))
}
