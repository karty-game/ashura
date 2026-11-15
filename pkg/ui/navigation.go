//nolint:mnd // Navigation limits, spacing and cyclic index arithmetic are explicit.
package ui

import (
	"slices"
	"unicode/utf8"
)

const maxTabs = 64

// Tab associates a stable key and label with retained panel content.
type Tab struct {
	Key, Label      string
	AccessibleLabel string
	Visual          ButtonVisual
	Content         *Node
	Disabled        bool
}

type tabGroup struct {
	items   []Tab
	buttons []*Node
	panels  []*Node
	active  int
}

// NewTabs builds a horizontal tab list with retained panels and one tab stop.
// Arrow keys activate immediately because all panel content is already mounted.
func NewTabs(label string, items []Tab, selected string, onChange func(Change)) *Node {
	root := Column()

	active, err := validateTabs(label, items, selected)
	if err != nil {
		root.constructionErr = err

		return root
	}

	group := &tabGroup{
		items:   append([]Tab(nil), items...),
		buttons: make([]*Node, 0, len(items)),
		panels:  make([]*Node, 0, len(items)),
		active:  -1,
	}
	root.tabs, root.label, root.change = group, label, onChange
	list := Row().WithStyle(Style{Gap: 6, Wrap: true})
	list.role, list.label = "tablist", label

	for index, item := range group.items {
		button := NewButton(item.Label, func() { root.activateTab(index) }).WithStyle(Style{FontSize: 14, Basis: 140, Grow: 1})
		if err := button.SetButtonVisual(item.Visual); err != nil {
			root.constructionErr = err

			return root
		}

		if err := button.SetLabel(item.AccessibleLabel, ""); err != nil {
			root.constructionErr = err

			return root
		}

		button.role, button.disabled = "tab", item.Disabled
		panel := Column(item.Content)
		panel.role, panel.labelledBy, panel.focusable = "tabpanel", button, !hasTabControl(item.Content)
		button.controls = panel
		group.buttons = append(group.buttons, button)
		group.panels = append(group.panels, panel)

		if err := list.Add(button); err != nil {
			root.constructionErr = err

			return root
		}
	}

	list.OnPreview(root.tabKey)
	root.constructionErr = root.Add(append([]*Node{list}, group.panels...)...)
	root.applyTab(active)

	return root
}

// ActiveTab returns the selected stable key, or an empty string for other nodes.
func (node *Node) ActiveTab() string {
	if node.tabs == nil || node.tabs.active < 0 {
		return ""
	}

	return node.tabs.items[node.tabs.active].Key
}

// SetActiveTab selects a panel without emitting an application change callback.
func (node *Node) SetActiveTab(key string) error {
	if node.tabs == nil {
		return ErrBounds
	}

	for index, item := range node.tabs.items {
		if item.Key == key && node.tabAvailable(index) {
			node.applyTab(index)

			return nil
		}
	}

	return ErrBounds
}

func validateTabs(label string, items []Tab, selected string) (int, error) {
	if !validNavigationString(label) || len(items) == 0 || len(items) > maxTabs {
		return -1, ErrBounds
	}

	keys, contents := map[string]bool{}, map[*Node]bool{}
	active, count := -1, 2+2*len(items)

	for index, item := range items {
		if !validTabItem(item) || keys[item.Key] || contents[item.Content] {
			return -1, ErrBounds
		}

		if err := validateNode(item.Content, 2, &count); err != nil {
			return -1, err
		}

		keys[item.Key], contents[item.Content] = true, true
		if !item.Disabled && (item.Key == selected || selected == "" && active < 0) {
			active = index
		}
	}

	if active < 0 {
		return -1, ErrBounds
	}

	return active, nil
}

func validTabItem(item Tab) bool {
	return item.Key != "" && validNavigationString(item.Key) && validNavigationString(item.Label) &&
		validNavigationString(item.AccessibleLabel) && validateButtonVisual(item.Visual) == nil &&
		(item.Label != "" || item.AccessibleLabel != "") && item.Content != nil &&
		item.Content.parent == nil &&
		item.Content.view == nil
}

func validNavigationString(text string) bool {
	return len(text) <= MaxTextBytes && utf8.ValidString(text)
}

func hasTabControl(node *Node) bool {
	if node.hidden || node.disabled {
		return false
	}

	if isControl(node.kind) || node.focusable {
		return !node.tabSkip
	}

	return slices.ContainsFunc(node.children, hasTabControl)
}

func (node *Node) tabAvailable(index int) bool {
	button := node.tabs.buttons[index]

	return !button.disabled && !button.hidden && button.parent != nil && button.parent.parent == node &&
		button.view == node.view && node.tabs.panels[index].view == node.view && node.tabs.panels[index].parent == node
}

func (node *Node) applyTab(index int) {
	group := node.tabs
	if group.active == index {
		return
	}

	view := node.view
	focusPanel := view != nil && group.active >= 0 && withinNode(view.focus, group.panels[group.active])
	group.active = index

	for current, button := range group.buttons {
		selected := current == index
		button.tabSkip, button.semanticSelected = !selected, selected
		button.SetChecked(selected)
		group.panels[current].SetHidden(!selected)
	}

	node.text = group.items[index].Key
	node.damage(true)

	if focusPanel && node.view == view {
		view.SetFocus(group.buttons[index])
	}
}

func withinNode(node, ancestor *Node) bool {
	for current := node; current != nil; current = current.parent {
		if current == ancestor {
			return true
		}
	}

	return false
}

func (node *Node) activateTab(index int) {
	if !node.tabAvailable(index) || node.tabs.active == index {
		return
	}

	view := node.view
	node.applyTab(index)

	if node.view == view {
		node.changeValue()
	}
}

func (node *Node) tabKey(event *RoutedEvent) {
	if event.Event.Kind != KeyDown || event.Event.Modifiers.Control || event.Event.Modifiers.Alt || event.Event.Modifiers.Meta {
		return
	}

	index := -1

	for current, button := range node.tabs.buttons {
		if button == event.Target {
			index = current
		}
	}

	if index < 0 {
		return
	}

	target := node.tabKeyTarget(index, event.Event.Key)
	if target < 0 {
		return
	}

	event.PreventDefault()

	view := node.view
	view.pending.Consumed = true
	view.focusVisible = true

	node.activateTab(target)

	if node.view == view {
		view.SetFocus(node.tabs.buttons[target])
		node.tabs.buttons[target].damage(false)
	}
}

func (node *Node) tabKeyTarget(index int, key string) int {
	step := 1

	switch key {
	case keyLeft, keyRight:
		if key == keyLeft {
			step = -1
		}
	case keyHome:
		index = -1
	case keyEnd:
		index, step = 0, -1
	default:
		return -1
	}

	for range len(node.tabs.buttons) {
		index = (index + step + len(node.tabs.buttons)) % len(node.tabs.buttons)
		if node.tabAvailable(index) {
			return index
		}
	}

	return -1
}
