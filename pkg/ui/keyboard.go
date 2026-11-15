//nolint:mnd // UI metrics, coordinate equations and bounded prepared-format widths are explicit.
package ui

import (
	"strings"
	"time"
)

func (v *View) keyboard(event Event) {
	if v.focus != nil && v.focus.composing {
		v.pending.Consumed = true

		return
	}

	if event.Kind == KeyUp {
		v.releaseKey(event)

		return
	}

	if event.Key == "Tab" {
		v.closePopup()
		v.focusVisible = true
		v.tab(event.Modifiers.Shift)
		v.pending.Consumed = true

		return
	}

	if event.Key == keyEscape && v.popup != nil {
		v.closePopup()
		v.pending.Consumed = true

		return
	}

	if event.Kind == KeyDown && event.Key == keyEscape && v.dismissTooltip() {
		v.pending.Consumed = true

		return
	}

	if event.Key == keyEscape && len(v.modals) > 0 {
		v.PopModal()
		v.pending.Consumed = true

		return
	}

	node := v.focus
	if node == nil {
		return
	}

	if !v.focusVisible {
		v.focusVisible = true

		node.damage(false)
	}

	//nolint:exhaustive // Each controller handles its declared subset; other event/control kinds are ignored or use the default.
	switch node.kind {
	case Input:
		v.editKey(node, event)
	case Select:
		v.pending.Consumed = v.selectKey(node, event)
	case Slider:
		v.rangeKey(node, event)
	default:
		if !v.scrollKey(node, event) {
			v.pressKey(node, event)
		}
	}
}

func (v *View) scrollKey(node *Node, event Event) bool {
	var delta float32

	switch event.Key {
	case "PageDown":
		delta = v.height * 0.8
	case "PageUp":
		delta = -v.height * 0.8
	case keyDown:
		delta = 40
	case keyUp:
		delta = -40
	case keyHome:
		delta = -32768

		for parent := node; parent != nil; parent = parent.parent {
			if parent.virtual != nil {
				delta = -parent.contentHeight

				break
			}
		}
	case keyEnd:
		delta = 32768

		for parent := node; parent != nil; parent = parent.parent {
			if parent.virtual != nil {
				delta = parent.contentHeight

				break
			}
		}
	default:
		return false
	}

	if v.scrollBy(node, delta) {
		v.pending.Consumed = true

		return true
	}

	return false
}

func (v *View) closePopup() {
	if v.popup != nil {
		node := v.popup
		v.popup = nil

		node.damage(false)
	}
}
func (v *View) nextOption(delta int) {
	node := v.popup
	if node == nil || len(node.options) == 0 {
		return
	}

	index := node.active
	if delta == 0 && index >= 0 && !node.options[index].Disabled {
		return
	}

	if delta == 0 {
		index = -1
		delta = 1
	}

	for range len(node.options) {
		index = (index + delta + len(node.options)) % len(node.options)
		if !node.options[index].Disabled {
			node.active = index

			v.popupNeedsReveal = true
			v.revealOption()
			node.damage(false)

			return
		}
	}

	node.active = -1
}
func (v *View) revealOption() {
	if v.popup == nil {
		return
	}

	top := float32(v.popup.active) * v.theme.ControlHeight

	bottom := top + v.theme.ControlHeight
	if top < v.popupOffset {
		v.popupOffset = top
	}

	if bottom > v.popupOffset+v.popupRect.H {
		v.popupOffset = max(0, bottom-v.popupRect.H)
	}
}
func (v *View) selectKey(node *Node, event Event) bool {
	if event.Repeat && (event.Key == keyEnter || event.Key == " ") {
		return true
	}

	if v.popup != node {
		if event.Key == keyDown || event.Key == keyUp || event.Key == keyEnter || event.Key == " " {
			v.activate(node)

			return true
		}

		return false
	}

	switch event.Key {
	case keyDown:
		v.nextOption(1)
	case keyUp:
		v.nextOption(-1)
	case keyHome:
		node.active = -1

		v.nextOption(1)
	case keyEnd:
		node.active = 0

		v.nextOption(-1)
	case keyEnter, " ":
		v.acceptOption()
	default:
		return v.selectTypeahead(node, event)
	}

	return true
}
func (v *View) acceptOption() {
	node := v.popup
	if node == nil {
		return
	}

	index := node.active

	v.closePopup()

	if index < 0 || index >= len(node.options) || node.options[index].Disabled || node.readOnly {
		return
	}

	old := node.selected
	node.selected = index
	node.damage(false)

	if old != index {
		node.changeValue()
	}

	v.finish(node)
}
func (v *View) popupExtent() float32 {
	if v.popup == nil {
		return 0
	}

	return max(0, float32(len(v.popup.options))*v.theme.ControlHeight-v.popupRect.H)
}
func (v *View) popupPointer(event Event) {
	index := int((event.Y - v.popupRect.Y + v.popupOffset) / v.theme.ControlHeight)
	if index >= 0 && index < len(v.popup.options) {
		if !v.popup.options[index].Disabled {
			v.popup.active = index
			v.popup.damage(false)
		}

		v.capture = &capture{
			node:      v.popup,
			pointer:   event.Pointer,
			popup:     true,
			option:    index,
			device:    event.Device,
			positionY: event.Y,
			lastY:     event.Y,
		}
		v.pending.Capture = true
		v.pending.Pointer = event.Pointer
	}

	v.pending.Consumed = true
}

func (v *View) releaseKey(event Event) {
	if v.keyPressed != event.Key || v.focus == nil {
		return
	}

	node := v.focus
	v.keyPressed = ""
	v.pressed = nil
	v.releaseControlRipple(node)

	node.damage(false)

	v.pending.Consumed = true
	if event.Key == " " {
		v.activate(node)
	}
}
func (v *View) pressKey(node *Node, event Event) {
	if event.Key != " " && event.Key != keyEnter {
		return
	}

	v.pending.Consumed = true

	if event.Repeat {
		return
	}

	if event.Key == keyEnter {
		if node.kind == Button || node.kind == ToggleButton {
			v.pulseControlRipple(node)
			v.activate(node)
		}

		return
	}

	v.keyPressed = event.Key
	v.pressed = node
	v.centerControlRipple(node)
	node.damage(false)
}
func (v *View) rangeKey(node *Node, event Event) {
	var delta float64

	switch event.Key {
	case keyLeft, keyDown:
		delta = -node.step
	case keyRight, keyUp:
		delta = node.step
	case "PageDown":
		delta = -node.step * 10
	case "PageUp":
		delta = node.step * 10
	case keyHome:
		delta = node.minimum - node.value
	case keyEnd:
		delta = node.maximum - node.value
	default:
		return
	}

	v.adjust(node, node.value+delta)
	v.finish(node)
	v.pending.Consumed = true
}

func (v *View) selectTypeahead(node *Node, event Event) bool {
	if len([]rune(event.Key)) != 1 || event.Modifiers.Control || event.Modifiers.Meta || event.Modifiers.Alt {
		return false
	}

	if event.Time-v.typeaheadAt > time.Second {
		v.typeahead = ""
	}

	v.typeahead += strings.ToLower(event.Key)
	v.typeaheadAt = event.Time

	for step := 1; step <= len(node.options); step++ {
		index := (node.active + step + len(node.options)) % len(node.options)
		if !node.options[index].Disabled && strings.HasPrefix(strings.ToLower(node.options[index].Label), v.typeahead) {
			node.active = index

			v.popupNeedsReveal = true
			v.revealOption()
			node.damage(false)

			break
		}
	}

	return true
}
