//nolint:mnd // UI metrics, coordinate equations and bounded prepared-format widths are explicit.
package ui

import (
	"fmt"
	"unicode/utf8"
)

func validateNode(node *Node, depth int, count *int) error {
	*count += 1

	if node.constructionErr != nil {
		return node.constructionErr
	}

	if *count > MaxNodes || depth > MaxDepth || node.kind > Chart || !validNodeStrings(node) {
		return ErrBounds
	}

	if err := node.SetStyle(node.style); err != nil {
		return err
	}

	if node.kind == Slider {
		if err := node.SetValue(node.value); err != nil {
			return err
		}
	}

	if err := validateOptions(node.options); err != nil {
		return err
	}

	if err := validateButtonVisual(node.buttonVisual); err != nil {
		return err
	}

	if err := validateVirtualList(node); err != nil {
		return err
	}

	if err := validateChartNode(node); err != nil {
		return err
	}

	if err := validatePanelBackground(node.panelBackground); err != nil {
		return err
	}

	for index, child := range node.children {
		if child == nil || child.parent != node {
			return ErrBounds
		}

		if err := validateNode(child, depth+1, count); err != nil {
			return fmt.Errorf("children[%d]: %w", index, err)
		}
	}

	return nil
}
func (v *View) attach(node *Node) {
	v.retainImage(nodeImage(node))

	v.next++
	node.id = v.next
	node.view = v

	v.nodes[node.id] = node
	for _, child := range node.children {
		v.attach(child)
	}
}
func (v *View) detach(node *Node) {
	v.releaseImage(nodeImage(node))

	if node.capacity > 0 {
		v.free = append(v.free, recordBlock{node.slot, node.capacity})
		node.capacity = 0
	}

	delete(v.nodes, node.id)

	node.motion = controlMotion{}

	node.view = nil
	for _, child := range node.children {
		v.detach(child)
	}
}
func (v *View) reconcile() {
	v.reconcileTabs()
	v.reconcileModals()

	if v.focus != nil && !v.focus.eligible() {
		v.SetFocus(nil)
	}

	if v.popup != nil && !v.popup.eligible() {
		v.closePopup()
	}

	if v.capture != nil && !v.validCapture() {
		v.cancelCapture()
	}
}
func (v *View) inScope(node *Node) bool {
	if len(v.modals) == 0 {
		return true
	}

	root := v.modals[len(v.modals)-1].root
	for parent := node; parent != nil; parent = parent.parent {
		if parent == root {
			return true
		}
	}

	return false
}
func (v *View) tab(back bool) {
	current := -1

	for index, node := range v.order {
		if node == v.focus {
			current = index
		}
	}

	step := 1
	if back {
		step = -1

		if current < 0 {
			current = len(v.order)
		}
	}

	for range len(v.order) {
		current = (current + step + len(v.order)) % len(v.order)

		node := v.order[current]
		if node.eligible() && !node.tabSkip && v.inScope(node) {
			v.SetFocus(node)

			return
		}
	}

	v.SetFocus(nil)
}
func (v *View) reveal(node *Node) {
	for parent := node.parent; parent != nil; parent = parent.parent {
		if !parent.style.Scroll {
			continue
		}

		v.stopScroll(parent)
		box := parent.rect.Inset(parent.style.Padding)

		delta := float32(0)
		if node.rect.Y < box.Y {
			delta = node.rect.Y - box.Y
		}

		if node.rect.Y+node.rect.H > box.Y+box.H {
			delta = node.rect.Y + node.rect.H - box.Y - box.H
		}

		if delta != 0 {
			v.setScrollOffset(parent, parent.offset+delta)
		}
	}
}

func validateOptions(options []Option) error {
	if len(options) > 1024 {
		return ErrBounds
	}

	keys := map[string]bool{}
	for _, option := range options {
		if option.Key == "" || len(option.Key) > MaxTextBytes || !utf8.ValidString(option.Key) || keys[option.Key] ||
			len(option.Label) > MaxTextBytes ||
			!utf8.ValidString(option.Label) {
			return ErrBounds
		}

		keys[option.Key] = true
	}

	return nil
}

func (v *View) reconcileModals() {
	for index, entry := range v.modals {
		if modalAvailable(entry.root, v) {
			continue
		}

		for len(v.modals) > index {
			root := v.modals[len(v.modals)-1].root
			hidden := root.hidden

			v.PopModal()

			if hidden {
				root.hidden = true
			}
		}

		return
	}
}
func modalAvailable(root *Node, v *View) bool {
	if root.view != v {
		return false
	}

	for parent := root; parent != nil; parent = parent.parent {
		if parent.hidden || parent.disabled || parent.virtualInactive {
			return false
		}
	}

	return true
}
func validateTheme(theme Theme) error {
	if err := validateMotionTheme(theme.Motion); err != nil {
		return err
	}

	for _, value := range []float32{theme.Radius, theme.FontSize, theme.ControlHeight, theme.Gap} {
		if !finite(value) || value < 0 || value > 32768 {
			return ErrBounds
		}
	}

	if theme.FontSize <= 0 || theme.FontSize > 256 || theme.ControlHeight <= 0 {
		return ErrBounds
	}

	for _, color := range []Color{theme.Background, theme.Surface, theme.Control, theme.Hover, theme.Pressed, theme.Foreground, theme.Muted, theme.Accent, theme.Focus, theme.Border, theme.Selection, theme.Info, theme.Success, theme.Warning, theme.Danger} {
		for _, value := range color {
			if !finite(value) || value < 0 || value > 1 {
				return ErrBounds
			}
		}
	}

	for _, palette := range []ButtonPalette{theme.PrimaryButton, theme.SecondaryButton} {
		for _, color := range []Color{palette.Fill, palette.Hover, palette.Pressed, palette.Foreground, palette.Border} {
			for _, value := range color {
				if !finite(value) || value < 0 || value > 1 {
					return ErrBounds
				}
			}
		}
	}

	return nil
}

func validNodeStrings(node *Node) bool {
	for _, text := range []string{node.text, node.label, node.placeholder, node.description, node.tooltip} {
		if len(text) > MaxTextBytes || !utf8.ValidString(text) {
			return false
		}
	}

	return true
}

func (v *View) validCapture() bool {
	node := v.capture.node
	if node.kind == Panel {
		return modalAvailable(node, v) && v.inScope(node) && (node.style.Scroll || node.focusable)
	}

	return node.eligible() && v.inScope(node)
}
