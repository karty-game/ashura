package ui

import (
	"math"
	"unicode/utf8"
)

const maxFontSize = 256

func NewPanel(children ...*Node) *Node {
	node := &Node{kind: Panel, selected: -1}
	node.constructionErr = node.Add(children...)

	return node
}
func Column(children ...*Node) *Node { return NewPanel(children...) }
func Row(children ...*Node) *Node {
	node := NewPanel(children...)
	node.style.Direction = Horizontal

	return node
}
func NewText(text string) *Node { return &Node{kind: Text, text: text} }
func NewButton(label string, activate func()) *Node {
	return &Node{kind: Button, text: label, activate: activate}
}
func NewCheckbox(label string, checked bool, change func(Change)) *Node {
	return &Node{kind: Checkbox, label: label, checked: checked, change: change}
}
func NewSwitch(label string, checked bool, change func(Change)) *Node {
	return &Node{kind: Switch, label: label, checked: checked, change: change}
}
func NewToggle(label string, checked bool, change func(Change)) *Node {
	return &Node{kind: ToggleButton, text: label, checked: checked, change: change}
}
func NewSlider(label string, minimum, maximum, step, value float64, change func(Change)) *Node {
	node := &Node{kind: Slider, label: label, minimum: minimum, maximum: maximum, step: step, change: change}
	node.constructionErr = node.SetValue(value)

	return node
}
func NewSelect(label string, options []Option, selected string, change func(Change)) *Node {
	node := &Node{kind: Select, label: label, options: append([]Option(nil), options...), selected: -1, active: -1, change: change}
	node.constructionErr = node.SetSelected(selected)

	return node
}
func NewInput(label, text, placeholder string, change func(Change)) *Node {
	return &Node{
		kind:        Input,
		revision:    1,
		label:       label,
		text:        text,
		placeholder: placeholder,
		change:      change,
		cursor:      len(text),
		anchor:      len(text),
	}
}

// Add rejects cycles, reparenting, foreign views and attached tree bounds atomically.
func (node *Node) Add(children ...*Node) error {
	if node.kind != Panel || node.virtual != nil {
		return invalid("Add", "Kind", node.kind, "children require a non-virtual panel")
	}

	seen := map[*Node]bool{}
	for index, child := range children {
		if child == nil || child.parent != nil || child.view != nil || seen[child] {
			return invalid("Add", "Child", index, "must be non-nil, detached and supplied only once")
		}

		for parent := node; parent != nil; parent = parent.parent {
			if parent == child {
				return invalid("Add", "Child", index, "would create a cycle")
			}
		}

		seen[child] = true
	}

	if node.view != nil {
		count := len(node.view.nodes)

		bytes := node.view.imageBytes + len(node.view.font.page.Pixels) + node.view.additionalImageBytes(children)

		if bytes > MaxImageBytes {
			return invalid("Add", "ImageBytes", bytes, "exceeds the view image budget")
		}

		depth := 0
		for parent := node; parent != nil; parent = parent.parent {
			depth++
		}

		for _, child := range children {
			if err := validateNode(child, depth, &count); err != nil {
				return err
			}
		}
	}

	for _, child := range children {
		child.parent = node

		node.children = append(node.children, child)
		if node.view != nil {
			node.view.attach(child)
		}
	}

	node.damage(true)

	return nil
}

// WithStyle configures construction, retaining the first error for Err, New or Add.
// Use SetStyle for mounted updates and receive validation errors immediately.
func (node *Node) WithStyle(style Style) *Node {
	return node.withError("WithStyle", node.SetStyle(style))
}

func (node *Node) SetStyle(style Style) error {
	if style.Direction == 0 {
		style.Direction = node.style.Direction
	}

	for _, field := range []struct {
		name  string
		value float32
	}{
		{"Basis", style.Basis}, {"Width", style.Width}, {"Height", style.Height},
		{"MinWidth", style.MinWidth}, {"MaxWidth", style.MaxWidth}, {"Grow", style.Grow},
		{"Padding", style.Padding}, {"Gap", style.Gap}, {"Radius", style.Radius},
		{"BorderWidth", style.BorderWidth}, {"FontSize", style.FontSize},
	} {
		if !finite(field.value) || field.value < 0 || field.value > 32768 {
			return invalid("SetStyle", field.name, field.value, "must be finite and between 0 and 32768")
		}
	}

	if node.virtual != nil {
		style.Scroll = true
	}

	if node.virtual != nil && (style.Direction == Horizontal || style.Wrap) {
		return invalid("SetStyle", "Direction", style.Direction, "virtual lists require vertical flow without wrapping")
	}

	if style.Direction > Horizontal {
		return invalid("SetStyle", "Direction", style.Direction, "must be Vertical or Horizontal")
	}

	if style.FontSize > maxFontSize {
		return invalid("SetStyle", "FontSize", style.FontSize, "must not exceed 256")
	}

	if style.MaxWidth > 0 && style.MinWidth > style.MaxWidth {
		return invalid("SetStyle", "MinWidth", style.MinWidth, "must not exceed MaxWidth")
	}

	for _, field := range []struct {
		name  string
		value Color
	}{{"Fill", style.Fill}, {"Foreground", style.Foreground}, {"Border", style.Border}} {
		for _, v := range field.value {
			if !finite(v) || v < 0 || v > 1 {
				return invalid("SetStyle", field.name, v, "channels must be finite and between 0 and 1")
			}
		}
	}

	if node.style != style {
		node.style = style
		node.damage(true)
	}

	return nil
}
func (node *Node) SetText(text string) error {
	if !utf8.ValidString(text) || len(text) > MaxTextBytes {
		return invalid("SetText", "TextBytes", len(text), "requires valid UTF-8 within MaxTextBytes")
	}

	if node.text != text {
		node.text = text
		node.revision++
		node.cursor = min(node.cursor, len(text))

		node.anchor = min(node.anchor, len(text))
		for node.cursor > 0 && !boundary(text, node.cursor) {
			node.cursor--
		}

		for node.anchor > 0 && !boundary(text, node.anchor) {
			node.anchor--
		}

		if node.composing {
			node.composing = false
		}

		node.damage(node.kind != Input)
	}

	return nil
}
func (node *Node) SetChecked(value bool) {
	if node.checked != value || node.mixed {
		node.checked = value
		node.mixed = false
		node.damage(false)
	}
}
func (node *Node) SetMixed(value bool) {
	if node.kind == Checkbox && node.mixed != value {
		node.mixed = value
		node.damage(false)
	}
}
func (node *Node) SetValue(value float64) error {
	if math.IsNaN(value) || math.IsInf(value, 0) || math.IsNaN(node.minimum) || math.IsNaN(node.maximum) || math.IsInf(node.minimum, 0) ||
		math.IsInf(node.maximum, 0) ||
		node.minimum > node.maximum ||
		node.step <= 0 ||
		math.IsNaN(node.step) ||
		math.IsInf(node.step, 0) {
		return ErrBounds
	}

	value = max(node.minimum, min(node.maximum, node.minimum+math.Round((value-node.minimum)/node.step)*node.step))
	if node.value != value {
		node.value = value
		node.damage(false)
	}

	return nil
}
func (node *Node) SetSelected(key string) error {
	index := -1

	for optionIndex, o := range node.options {
		if o.Key == key {
			index = optionIndex

			break
		}
	}

	if index < 0 && key != "" {
		return invalid("SetSelected", "SelectedBytes", len(key), "key must match a declared option")
	}

	if node.selected != index {
		node.selected = index
		node.active = index
		node.damage(false)
	}

	return nil
}
func (node *Node) SetDisabled(value bool) {
	if node.disabled != value {
		node.disabled = value
		node.damage(false)

		if node.view != nil {
			node.view.reconcile()
		}
	}
}
func (node *Node) SetHidden(value bool) {
	if node.hidden != value {
		node.hidden = value
		node.damage(true)

		if node.view != nil {
			node.view.reconcile()
		}
	}
}
func (node *Node) SetReadOnly(value bool) {
	if node.readOnly != value {
		node.readOnly = value
		if value && node.composing && node.view != nil {
			node.view.cancelComposition(node)
		}

		node.damage(false)
	}
}
func (node *Node) SetLabel(label, description string) error {
	if len(label) > MaxTextBytes || len(description) > MaxTextBytes || !utf8.ValidString(label) || !utf8.ValidString(description) {
		return invalid("SetLabel", "TextBytes", max(len(label), len(description)), "requires valid UTF-8 within MaxTextBytes")
	}

	if node.label != label || node.description != description {
		node.label, node.description = label, description
		node.damage(true)
	}

	return nil
}
func (node *Node) OnCommit(callback func(Change))        { node.commit = callback }
func (node *Node) OnCancel(callback func())              { node.cancel = callback }
func (node *Node) OnPreview(callback func(*RoutedEvent)) { node.preview = callback }
func (node *Node) OnEvent(callback func(*RoutedEvent))   { node.bubble = callback }

// SetFocusable includes a Panel in keyboard navigation, for example a scrollable
// statistics region. Controls are focusable automatically and reject this option.
func (node *Node) SetFocusable(value bool) error {
	if node.kind != Panel {
		return ErrBounds
	}

	if node.focusable != value {
		node.focusable = value
		node.damage(false)

		if node.view != nil {
			node.view.reconcile()
		}
	}

	return nil
}

func (node *Node) Remove() {
	if node.parent == nil {
		if node.view != nil {
			node.view.removeOverlayNode(node)
		}

		return
	}

	if node.parent.virtual != nil {
		return
	}

	parent := node.parent
	for index, child := range parent.children {
		if child == node {
			copy(parent.children[index:], parent.children[index+1:])
			parent.children[len(parent.children)-1] = nil
			parent.children = parent.children[:len(parent.children)-1]

			break
		}
	}

	if node.view != nil {
		v := node.view
		v.detach(node)
		v.layoutDirty = true
		v.reconcile()
	}

	node.parent = nil

	parent.damage(true)
}
func (node *Node) damage(layout bool) {
	if node.view == nil {
		return
	}

	view := node.view
	view.layoutDirty = view.layoutDirty || layout

	if !node.dirty {
		node.dirty = true
		if layout || node.layoutRevision == view.layoutRevision {
			view.dirty = append(view.dirty, node)
		}
	}
}
func (node *Node) eligible() bool {
	if node.view == nil || (!isControl(node.kind) && (node.kind != Panel || !node.focusable)) {
		return false
	}

	for parent := node; parent != nil; parent = parent.parent {
		if parent.disabled || parent.hidden || parent.virtualInactive {
			return false
		}
	}

	return true
}
func (node *Node) changeValue() {
	if node.change != nil {
		node.change(node.changed())
	}
}
func (node *Node) changed() Change {
	text := node.text
	if node.kind == Select {
		text = node.Selected()
	}

	return Change{Node: node, Text: text, Value: node.value, Checked: node.checked, Composing: node.composing}
}
